package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
	"g9router/internal/translator"
	"g9router/internal/usage"
	"g9router/internal/web"
)

type Options struct {
	Addr, Upstream, APIKey, ProviderPath, OAuthPath string
}

type Server struct {
	options Options
	client  *http.Client
	store   *providers.Store
	usage   *usage.Store
	oauth   *oauth.Manager
}

func New(options Options) *Server {
	if options.Upstream == "" {
		options.Upstream = "https://api.openai.com/v1"
	}
	if options.ProviderPath == "" {
		options.ProviderPath = "providers.json"
	}
	if options.OAuthPath == "" {
		options.OAuthPath = "oauth.json"
	}
	return &Server{options: options, client: &http.Client{Timeout: 10 * time.Minute}, store: providers.New(options.ProviderPath), usage: &usage.Store{}, oauth: oauth.New(options.OAuthPath)}
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
	mux.HandleFunc("/api/providers", s.providerAPI)
	mux.HandleFunc("/api/usage", s.usageAPI)
	mux.HandleFunc("/api/oauth", s.oauthAPI)
	mux.Handle("/", web.Handler())
	return logging(mux)
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
	s.proxy(w, r, s.options.Upstream, "/models", http.MethodGet, nil, "")
}

func (s *Server) chatCompletions(w http.ResponseWriter, r *http.Request) {
	s.forwardJSON(w, r, "/chat/completions")
}

func (s *Server) responses(w http.ResponseWriter, r *http.Request) { s.forwardJSON(w, r, "/responses") }
func (s *Server) messages(w http.ResponseWriter, r *http.Request)  { s.forwardJSON(w, r, "/messages") }

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
