package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"g9router/internal/auth"
	"g9router/internal/chatcore"
	"g9router/internal/db"
	"g9router/internal/executor"
	"g9router/internal/format"
	"g9router/internal/oauth"
	"g9router/internal/oidc"
	"g9router/internal/providers"
	"g9router/internal/rtk"
	"g9router/internal/settings"
	"g9router/internal/translator"
	"g9router/internal/usage"
	"g9router/internal/web"
)

type Options struct {
	Addr, Upstream, APIKey, ProviderPath, OAuthPath string
}

type Server struct {
	options    Options
	client     *http.Client
	store      *providers.Store
	usage      *usage.Store
	oauth      *oauth.Manager
	settings   *settings.Store
	sessions   *auth.Sessions
	oidcConfig oidc.Config
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
	return &Server{options: options, client: &http.Client{Timeout: 10 * time.Minute}, store: providers.New(options.ProviderPath), usage: usage.New("g9router.db"), oauth: oauth.New(options.OAuthPath), settings: settings.New(database), sessions: auth.NewSessions(), oidcConfig: oidc.ConfigFromEnv(os.Getenv)}
}

func (s *Server) Run() error {
	log.Printf("g9router listening on %s, upstream %s", s.options.Addr, s.options.Upstream)
	return http.ListenAndServe(s.options.Addr, s.Handler())
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/v1/models", s.models)
	mux.HandleFunc("/v1beta/models", s.betaModels)
	mux.HandleFunc("/v1/chat/completions", s.chatCompletions)
	mux.HandleFunc("/v1/responses", s.responses)
	mux.HandleFunc("/v1/messages", s.messages)
	mux.HandleFunc("/v1/embeddings", s.embeddings)
	mux.HandleFunc("/v1/images/generations", s.images)
	mux.HandleFunc("/v1/audio/transcriptions", s.transcriptions)
	mux.HandleFunc("/v1/audio/speech", s.speech)
	mux.HandleFunc("/api/providers", s.providerAPI)
	mux.HandleFunc("/api/providers/", s.providerResourceAPI)
	mux.HandleFunc("/api/providers/client", s.providerClientAPI)
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/usage", s.usageAPI)
	mux.HandleFunc("/api/oauth", s.oauthAPI)
	mux.HandleFunc("/api/settings", s.settingsAPI)
	mux.HandleFunc("/api/models/alias", s.modelAliasAPI)
	mux.HandleFunc("/api/models/custom", s.customModelsAPI)
	mux.HandleFunc("/api/models/disabled", s.disabledModelsAPI)
	mux.HandleFunc("/api/auth/status", s.authStatus)
	mux.HandleFunc("/api/auth/login", s.authLogin)
	mux.HandleFunc("/api/auth/logout", s.authLogout)
	mux.HandleFunc("/api/auth/oidc/start", s.oidcStart)
	mux.HandleFunc("/api/auth/oidc/callback", s.oidcCallback)
	mux.Handle("/", web.Handler())
	return logging(mux)
}

func (s *Server) providerResourceAPI(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/providers/"), "/")
	if strings.HasSuffix(id, "/test") {
		s.providerTestAPI(w, r, strings.TrimSuffix(id, "/test"))
		return
	}
	if strings.HasSuffix(id, "/models") {
		s.providerModelsAPI(w, r, strings.TrimSuffix(id, "/models"))
		return
	}
	if id == "" || id == "client" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}
	if r.Method == http.MethodDelete {
		if err := s.store.Delete(id); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "deleted"})
		return
	}
	if r.Method == http.MethodGet {
		for _, provider := range s.store.List() {
			if provider.ID == id {
				writeJSON(w, 200, provider)
				return
			}
		}
		writeJSON(w, 404, map[string]string{"error": "provider not found"})
		return
	}
	if r.Method == http.MethodPut {
		var provider providers.Provider
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&provider) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		provider.ID = id
		if provider.BaseURL == "" {
			writeJSON(w, 400, map[string]string{"error": "baseURL is required"})
			return
		}
		if err := s.store.Upsert(provider); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "saved"})
		return
	}
	writeJSON(w, 405, map[string]string{"error": "method not allowed"})
}

func (s *Server) providerTestAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider, found := s.store.Find(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(provider.BaseURL, "/")+"/models", nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	if provider.APIType == "claude" || provider.APIType == "anthropic" {
		request.Header.Set("x-api-key", provider.APIKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else if provider.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error(), "refreshed": false})
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		writeJSON(w, http.StatusOK, map[string]any{"valid": true, "error": nil, "refreshed": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": fmt.Sprintf("upstream status %d", response.StatusCode), "refreshed": false})
}

func (s *Server) providerModelsAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider, found := s.store.Find(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(provider.BaseURL, "/")+"/models", nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if provider.APIType == "claude" || provider.APIType == "anthropic" {
		request.Header.Set("x-api-key", provider.APIKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else if provider.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	token, _ := r.Cookie("g9router_session")
	writeJSON(w, 200, map[string]any{"authenticated": token != nil && s.sessions.Valid(token.Value), "passwordConfigured": os.Getenv("G9ROUTER_PASSWORD") != ""})
}
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.Password == "" {
		writeJSON(w, 400, map[string]string{"error": "password is required"})
		return
	}
	expected := os.Getenv("G9ROUTER_PASSWORD")
	if expected == "" || input.Password != expected {
		writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		writeJSON(w, 500, map[string]string{"error": "session generation failed"})
		return
	}
	token := hex.EncodeToString(raw)
	s.sessions.Create(token)
	http.SetCookie(w, &http.Cookie{Name: "g9router_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400})
	writeJSON(w, 200, map[string]bool{"success": true})
}
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("g9router_session"); err == nil {
		s.sessions.Delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "g9router_session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"success": true})
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	if !s.oidcConfig.Enabled() {
		http.Error(w, "OIDC is not configured", http.StatusNotFound)
		return
	}
	state := oidc.NewState()
	http.SetCookie(w, &http.Cookie{Name: "g9router_oidc_state", Value: state, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	location, err := s.oidcConfig.AuthorizationURL(r.Context(), state)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, location, http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("g9router_oidc_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid OIDC state", http.StatusBadRequest)
		return
	}
	if _, err := s.oidcConfig.Exchange(r.Context(), r.URL.Query().Get("code")); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	token := hex.EncodeToString(raw)
	s.sessions.Create(token)
	http.SetCookie(w, &http.Cookie{Name: "g9router_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400})
	http.Redirect(w, r, "/", http.StatusFound)
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

func (s *Server) modelAliasAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"aliases": s.settings.ModelAliases()})
	case http.MethodPut:
		var input struct {
			Model string `json:"model"`
			Alias string `json:"alias"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.Model == "" || input.Alias == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Model and alias required"})
			return
		}
		if err := s.settings.SetModelAlias(input.Alias, input.Model); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "model": input.Model, "alias": input.Alias})
	case http.MethodDelete:
		alias := r.URL.Query().Get("alias")
		if alias == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Alias required"})
			return
		}
		if err := s.settings.DeleteModelAlias(alias); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) customModelsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"models": s.settings.CustomModels()})
	case http.MethodPost:
		var input map[string]any
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		provider, _ := input["providerAlias"].(string)
		id, _ := input["id"].(string)
		if provider == "" || id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "providerAlias and id required"})
			return
		}
		if input["type"] == nil {
			input["type"] = "llm"
		}
		if _, err := s.settings.AddCustomModel(input); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "added": input})
	case http.MethodDelete:
		provider, id, kind := r.URL.Query().Get("providerAlias"), r.URL.Query().Get("id"), r.URL.Query().Get("type")
		if kind == "" {
			kind = "llm"
		}
		if provider == "" || id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "providerAlias and id required"})
			return
		}
		if err := s.settings.DeleteCustomModel(provider, id, kind); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) disabledModelsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		all := s.settings.DisabledModels()
		provider := r.URL.Query().Get("providerAlias")
		if provider != "" {
			writeJSON(w, http.StatusOK, map[string]any{"ids": all[provider]})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"disabled": all})
	case http.MethodPost:
		var input struct {
			Provider string   `json:"providerAlias"`
			IDs      []string `json:"ids"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.Provider == "" || input.IDs == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "providerAlias and ids[] required"})
			return
		}
		if err := s.settings.SetDisabledModels(input.Provider, input.IDs); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case http.MethodDelete:
		provider := r.URL.Query().Get("providerAlias")
		if provider == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "providerAlias required"})
			return
		}
		ids := []string{}
		if id := r.URL.Query().Get("id"); id != "" {
			ids = append(ids, id)
		}
		if err := s.settings.EnableModels(provider, ids); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
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
func (s *Server) betaModels(w http.ResponseWriter, r *http.Request) { s.models(w, r) }
func (s *Server) providerClientAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, 200, map[string]any{"providers": s.store.List(), "baseURL": "/v1"})
}

func (s *Server) aggregateModels(w http.ResponseWriter, r *http.Request) bool {
	type modelList struct {
		Data []map[string]any `json:"data"`
	}
	merged := map[string]map[string]any{}
	disabled := s.settings.DisabledModels()
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
							if containsString(disabled[provider.ID], id) {
								continue
							}
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
	for _, model := range s.settings.CustomModels() {
		id, _ := model["id"].(string)
		provider, _ := model["providerAlias"].(string)
		if id != "" && !containsString(disabled[provider], id) {
			if _, exists := merged[id]; !exists {
				merged[id] = map[string]any{"id": id, "object": "model", "owned_by": provider, "name": model["name"], "type": model["type"]}
			}
		}
	}
	data := make([]map[string]any, 0, len(merged))
	for _, model := range merged {
		data = append(data, model)
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
	if os.Getenv("G9ROUTER_RTK") == "1" {
		rtk.CompressMessages(arrayValue(request["messages"]))
		body, _ = json.Marshal(request)
	}
	if messages, ok := request["messages"].([]any); ok {
		request["messages"] = chatcore.FixMissingToolResponses(messages)
		if tools, ok := request["tools"].([]any); ok {
			request["tools"] = chatcore.DedupeTools(tools).Tools
		}
		request = chatcore.NormalizeThinking(request, os.Getenv("G9ROUTER_THINKING_MODE"))
		body, _ = json.Marshal(request)
	}
	model, _ := request["model"].(string)
	if target := s.settings.ModelAliases()[model]; target != "" {
		request["model"] = target
		model = target
		body, _ = json.Marshal(request)
	}
	sourceFormat := format.Detect(request)
	providers := s.store.Resolve(model)
	if len(providers) == 0 {
		s.proxy(w, r, s.options.Upstream, path, http.MethodPost, body, s.options.APIKey)
		return
	}
	for _, provider := range providers {
		providerBody := body
		providerPath := path
		translateResponse := false
		if path == "/responses" && provider.APIType == "openai-chat" {
			var responsesBody map[string]any
			if json.Unmarshal(body, &responsesBody) == nil {
				translated, _ := json.Marshal(translator.ResponsesToChat(responsesBody))
				if s.proxyResponses(w, r, provider.BaseURL, translated, provider.APIKey) {
					return
				}
				continue
			}
		}
		if path == "/responses" && (provider.APIType == "codex" || provider.APIType == "openai-responses") {
			var responsesBody map[string]any
			if json.Unmarshal(body, &responsesBody) == nil {
				providerBody, _ = json.Marshal(translator.NormalizeCodexRequest(responsesBody))
				if s.proxy(w, r, provider.BaseURL, "/responses", http.MethodPost, providerBody, provider.APIKey) {
					return
				}
				continue
			}
		}
		if provider.APIType == "gemini" {
			if s.proxyGemini(w, r, provider.BaseURL, model, request, provider.APIKey) {
				return
			}
			continue
		}
		if sourceFormat == format.Claude && provider.APIType == "openai" {
			translateResponse = true
			providerPath = "/chat/completions"
			var claude map[string]any
			if json.Unmarshal(body, &claude) == nil {
				stream, _ := claude["stream"].(bool)
				model, _ := claude["model"].(string)
				translated, _ := json.Marshal(translator.ClaudeToOpenAI(model, claude, stream))
				providerBody = translated
			}
		}
		if sourceFormat == format.OpenAI && (provider.APIType == "claude" || provider.APIType == "anthropic" || (provider.APIType == "github" && strings.HasPrefix(strings.ToLower(model), "claude-"))) {
			var openAI map[string]any
			if json.Unmarshal(body, &openAI) == nil {
				stream, _ := openAI["stream"].(bool)
				providerBody, _ = json.Marshal(translator.OpenAIToClaudeRequest(model, openAI, stream))
				providerPath = "/messages"
				if provider.APIType == "github" {
					providerPath = "/v1/messages"
				}
				if s.proxyTranslatedResponse(w, r, provider.BaseURL, providerPath, providerBody, provider.APIKey, true) {
					return
				}
				continue
			}
		}
		if translateResponse {
			if s.proxyTranslated(w, r, provider.BaseURL, providerPath, providerBody, provider.APIKey) {
				return
			}
			continue
		}
		if s.proxy(w, r, provider.BaseURL, providerPath, http.MethodPost, providerBody, provider.APIKey) {
			return
		}
	}
	s.usage.Add(0, 1, int64(len(body)), 0)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "all providers failed"})
}

func arrayValue(value any) []any { values, _ := value.([]any); return values }

func (s *Server) proxyResponses(w http.ResponseWriter, incoming *http.Request, baseURL string, body []byte, apiKey string) bool {
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
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
	var chat map[string]any
	if json.NewDecoder(response.Body).Decode(&chat) != nil {
		return false
	}
	writeJSON(w, response.StatusCode, translator.ChatToResponsesResponse(chat))
	return true
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
	return s.proxyTranslatedResponse(w, incoming, baseURL, path, body, apiKey, false)
}

func (s *Server) proxyTranslatedResponse(w http.ResponseWriter, incoming *http.Request, baseURL, path string, body []byte, apiKey string, claudeResponse bool) bool {
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
		if claudeResponse {
			return s.proxyClaudeToOpenAIStream(w, response)
		}
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
	if claudeResponse {
		translated = translator.ClaudeResponseToOpenAI(openAI)
	}
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

func (s *Server) proxyClaudeToOpenAIStream(w http.ResponseWriter, response *http.Response) bool {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(response.StatusCode)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	state := &translator.ClaudeStreamState{}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		for _, output := range translator.ClaudeChunkToOpenAISSE([]byte(payload), state) {
			_, _ = io.WriteString(w, output)
			flusher.Flush()
		}
	}
	return scanner.Err() == nil
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
	if method == http.MethodPost {
		return s.proxyWithExecutor(w, incoming, baseURL, path, body, apiKey)
	}
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

func (s *Server) proxyWithExecutor(w http.ResponseWriter, incoming *http.Request, baseURL, path string, body []byte, apiKey string) bool {
	headers := map[string]string{}
	if authorization := incoming.Header.Get("Authorization"); authorization != "" {
		headers["Authorization"] = authorization
	}
	result, err := executor.Execute(incoming.Context(), s.client, executor.Config{BaseURLs: []string{baseURL}, Headers: headers, APIKey: apiKey, RetryAttempts: 2, RetryDelay: 250 * time.Millisecond}, path, body, incoming.Header.Get("Accept") == "text/event-stream")
	if err != nil {
		return false
	}
	for key, values := range result.Header {
		if key == "Content-Length" || key == "Content-Encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(result.Status)
	_, _ = w.Write(result.Body)
	return result.Status < 500
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
