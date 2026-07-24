package server

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"g9router/internal/db"
	"g9router/internal/oauth"
	"g9router/internal/providers"
	"g9router/internal/settings"
	"g9router/internal/translator"
	"g9router/internal/usage"
	"g9router/internal/web"
)

type Options struct {
	Addr, Upstream, APIKey, ProviderPath, OAuthPath string
}

type Server struct {
	options  Options
	client   *http.Client
	store    *providers.Store
	usage    *usage.Store
	oauth    *oauth.Manager
	settings *settings.Store
}

func New(options Options) *Server {
	if options.Upstream == "" {
		options.Upstream = "https://api.openai.com/v1"
	}
	if options.ProviderPath == "" {
		options.ProviderPath = "g9router.db"
	}
	if options.OAuthPath == "" {
		options.OAuthPath = "g9router.db"
	}
	var database *sql.DB
	if opened, err := db.Open("g9router.db"); err == nil {
		database = opened
	}
	return &Server{options: options, client: &http.Client{Timeout: 10 * time.Minute}, store: providers.New(options.ProviderPath), usage: usage.New("g9router.db"), oauth: oauth.New(options.OAuthPath), settings: settings.New(database)}
}

func (s *Server) Run() error {
	log.Printf("g9router listening on %s, upstream %s", s.options.Addr, s.options.Upstream)
	return http.ListenAndServe(s.options.Addr, s.Handler())
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/v1/models", s.models)
	mux.HandleFunc("/v1/chat/completions", s.chatCompletions)
	mux.HandleFunc("/v1/responses", s.responses)
	mux.HandleFunc("/v1/messages", s.messages)
	mux.HandleFunc("/v1/embeddings", s.embeddings)
	mux.HandleFunc("/v1/images/generations", s.images)
	mux.HandleFunc("/v1/audio/transcriptions", s.transcriptions)
	mux.HandleFunc("/v1/audio/speech", s.speech)
	mux.HandleFunc("/api/providers", s.providerAPI)
	mux.HandleFunc("/api/usage", s.usageAPI)
	mux.HandleFunc("/api/oauth", s.oauthAPI)
	mux.HandleFunc("/api/settings", s.settingsAPI)
	mux.Handle("/", web.Handler())
	return logging(mux)
}

func (s *Server) settingsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.settings.Get())
	case http.MethodPut, http.MethodPost:
		var values map[string]any
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&values) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.settings.Update(values); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "saved"})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) oauthAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.oauth.List())
	case http.MethodPost:
		var credential oauth.Credential
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&credential); err != nil || credential.ID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}
		if err := s.oauth.Upsert(credential); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	case http.MethodPut:
		id := r.URL.Query().Get("id")
		credential, err := s.oauth.Refresh(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		credential.AccessToken = ""
		credential.RefreshToken = ""
		writeJSON(w, http.StatusOK, credential)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) usageAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		if err := s.usage.Reset(); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "reset"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, s.usage.Snapshot())
}

func (s *Server) providerAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.List())
	case http.MethodPost:
		var provider providers.Provider
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&provider); err != nil || provider.ID == "" || provider.BaseURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and baseURL are required"})
			return
		}
		if err := s.store.Upsert(provider); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
	case http.MethodDelete:
		if err := s.store.Delete(r.URL.Query().Get("id")); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if s.aggregateModels(w, r) {
			return
		}
	}
	s.proxy(w, r, s.options.Upstream, "/models", http.MethodGet, nil, "")
}

func (s *Server) aggregateModels(w http.ResponseWriter, r *http.Request) bool {
	type modelList struct {
		Data []map[string]any `json:"data"`
	}
	merged := map[string]map[string]any{}
	sources := append([]providers.Provider{{BaseURL: s.options.Upstream, APIKey: s.options.APIKey, Enabled: true}}, s.store.Enabled()...)
	for _, provider := range sources {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(provider.BaseURL, "/")+"/models", nil)
		if err == nil {
			if provider.APIKey != "" {
				req.Header.Set("Authorization", "Bearer "+provider.APIKey)
			}
			resp, doErr := s.client.Do(req)
			if doErr == nil && resp.StatusCode < 400 {
				var payload modelList
				if json.NewDecoder(resp.Body).Decode(&payload) == nil {
					for _, model := range payload.Data {
						if id, ok := model["id"].(string); ok && id != "" {
							if _, exists := merged[id]; !exists {
								merged[id] = model
							}
						}
					}
				}
				resp.Body.Close()
			}
		}
		cancel()
	}
	data := make([]map[string]any, 0, len(merged))
	for _, model := range merged {
		data = append(data, model)
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
	return true
}

func (s *Server) chatCompletions(w http.ResponseWriter, r *http.Request) {
	s.forwardJSON(w, r, "/chat/completions")
}

func (s *Server) responses(w http.ResponseWriter, r *http.Request) { s.forwardJSON(w, r, "/responses") }
func (s *Server) messages(w http.ResponseWriter, r *http.Request)  { s.forwardJSON(w, r, "/messages") }
func (s *Server) embeddings(w http.ResponseWriter, r *http.Request) {
	s.forwardRaw(w, r, "/embeddings")
}
func (s *Server) images(w http.ResponseWriter, r *http.Request) {
	s.forwardRaw(w, r, "/images/generations")
}
func (s *Server) transcriptions(w http.ResponseWriter, r *http.Request) {
	s.forwardRaw(w, r, "/audio/transcriptions")
}
func (s *Server) speech(w http.ResponseWriter, r *http.Request) { s.forwardRaw(w, r, "/audio/speech") }

func (s *Server) forwardRaw(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
		return
	}
	if !s.proxy(w, r, s.options.Upstream, path, http.MethodPost, body, s.options.APIKey) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream unavailable"})
	}
}

func (s *Server) forwardJSON(w http.ResponseWriter, r *http.Request, path string) {
	s.usage.Add(1, 0, 0, 0)
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
		return
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if _, ok := request["model"]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
		return
	}
	model, _ := request["model"].(string)
	providers := s.store.Resolve(model)
	if len(providers) == 0 {
		s.proxy(w, r, s.options.Upstream, path, http.MethodPost, body, s.options.APIKey)
		return
	}
	for _, provider := range providers {
		providerBody := body
		translateResponse := false
		if provider.APIType == "gemini" {
			if s.proxyGemini(w, r, provider.BaseURL, model, request, provider.APIKey) {
				return
			}
			continue
		}
		if path == "/messages" && provider.APIType == "openai" {
			translateResponse = true
			var claude map[string]any
			if json.Unmarshal(body, &claude) == nil {
				stream, _ := claude["stream"].(bool)
				model, _ := claude["model"].(string)
				translated, _ := json.Marshal(translator.ClaudeToOpenAI(model, claude, stream))
				providerBody = translated
			}
		}
		if translateResponse {
			if s.proxyTranslated(w, r, provider.BaseURL, path, providerBody, provider.APIKey) {
				return
			}
			continue
		}
		if s.proxy(w, r, provider.BaseURL, path, http.MethodPost, providerBody, provider.APIKey) {
			return
		}
	}
	s.usage.Add(0, 1, int64(len(body)), 0)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "all providers failed"})
}

func (s *Server) proxyGemini(w http.ResponseWriter, incoming *http.Request, baseURL, model string, request map[string]any, apiKey string) bool {
	body := translator.OpenAIToGemini(model, request)
	encoded, err := json.Marshal(body)
	if err != nil {
		return false
	}
	endpoint := strings.TrimRight(baseURL, "/")
	if strings.Contains(endpoint, "{model}") {
		endpoint = strings.ReplaceAll(endpoint, "{model}", model)
	} else if !strings.HasSuffix(endpoint, ":generateContent") {
		endpoint += "/models/" + model + ":generateContent"
	}
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-Goog-Api-Key", apiKey)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return false
	}
	var gemini map[string]any
	if json.Unmarshal(raw, &gemini) != nil {
		return false
	}
	result := translator.GeminiToOpenAI(model, gemini)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_ = json.NewEncoder(w).Encode(result)
	return true
}

func (s *Server) proxyTranslated(w http.ResponseWriter, incoming *http.Request, baseURL, path string, body []byte, apiKey string) bool {
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	if key := incoming.Header.Get("Authorization"); key != "" {
		request.Header.Set("Authorization", key)
	} else if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return false
	}
	if incoming.Header.Get("Accept") == "text/event-stream" {
		return s.proxyClaudeStream(w, response)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return false
	}
	var openAI map[string]any
	if json.Unmarshal(raw, &openAI) != nil {
		return false
	}
	translated := translator.OpenAIToClaudeResponse(openAI)
	for key, values := range response.Header {
		if key == "Content-Length" || key == "Content-Encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_ = json.NewEncoder(w).Encode(translated)
	return true
}

func (s *Server) proxyClaudeStream(w http.ResponseWriter, response *http.Response) bool {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(response.StatusCode)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	state := &translator.StreamState{}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			continue
		}
		for _, output := range translator.OpenAIChunkToClaudeSSE([]byte(payload), state) {
			_, _ = io.WriteString(w, output)
			flusher.Flush()
		}
	}
	return scanner.Err() == nil
}

func (s *Server) proxy(w http.ResponseWriter, incoming *http.Request, baseURL, path, method string, body []byte, apiKey string) bool {
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, reader)
	if err != nil {
		return false
	}
	request.Header.Set("Accept", incoming.Header.Get("Accept"))
	request.Header.Set("Content-Type", "application/json")
	if key := incoming.Header.Get("Authorization"); key != "" {
		request.Header.Set("Authorization", key)
	} else if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return false
	}
	for key, values := range response.Header {
		if key == "Content-Length" || key == "Content-Encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if response.Header.Get("Content-Type") == "text/event-stream" {
		if flusher, ok := w.(http.Flusher); ok {
			streamCopy(w, response.Body, flusher)
			return true
		}
	}
	_, _ = io.Copy(w, response.Body)
	return true
}

func streamCopy(w io.Writer, body io.Reader, flusher http.Flusher) {
	buffer := make([]byte, 32<<10)
	for {
		count, err := body.Read(buffer)
		if count > 0 {
			_, _ = w.Write(buffer[:count])
			flusher.Flush()
		}
		if errors.Is(err, io.EOF) || err != nil {
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}
