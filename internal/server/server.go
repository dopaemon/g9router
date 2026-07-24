package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"g9router/internal/auth"
	"g9router/internal/chatcore"
	"g9router/internal/cursor"
	"g9router/internal/db"
	"g9router/internal/executor"
	"g9router/internal/format"
	"g9router/internal/headroom"
	"g9router/internal/mcp"
	"g9router/internal/oauth"
	"g9router/internal/oidc"
	"g9router/internal/providers"
	"g9router/internal/rtk"
	"g9router/internal/settings"
	"g9router/internal/translator"
	"g9router/internal/usage"
	"g9router/internal/vertex"
	"g9router/internal/web"
)

type Options struct {
	Addr, Upstream, APIKey, ProviderPath, OAuthPath string
}

type Server struct {
	options         Options
	client          *http.Client
	store           *providers.Store
	usage           *usage.Store
	oauth           *oauth.Manager
	settings        *settings.Store
	sessions        *auth.Sessions
	oidcConfig      oidc.Config
	mcpBridge       *mcp.Bridge
	headroomManager *headroom.Manager
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
	return &Server{options: options, client: &http.Client{Timeout: 10 * time.Minute}, store: providers.New(options.ProviderPath), usage: usage.New("g9router.db"), oauth: oauth.New(options.OAuthPath), settings: settings.New(database), sessions: auth.NewSessions(), oidcConfig: oidc.ConfigFromEnv(os.Getenv), mcpBridge: mcp.New(), headroomManager: headroom.New(os.Getenv("G9ROUTER_HEADROOM_COMMAND"))}
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
	mux.HandleFunc("/api/models/availability", s.modelsAvailabilityAPI)
	mux.HandleFunc("/api/cli-tools/all-statuses", s.cliToolsStatusAPI)
	mux.HandleFunc("/api/cli-tools/claude-settings", s.claudeSettingsAPI)
	mux.HandleFunc("/api/cli-tools/codex-settings", s.codexSettingsAPI)
	mux.HandleFunc("/api/cli-tools/opencode-settings", s.opencodeSettingsAPI)
	mux.HandleFunc("/api/cli-tools/copilot-settings", s.copilotSettingsAPI)
	mux.HandleFunc("/api/cli-tools/droid-settings", s.droidSettingsAPI)
	mux.HandleFunc("/api/mcp/", s.mcpAPI)
	mux.HandleFunc("/api/headroom/status", s.headroomStatusAPI)
	mux.HandleFunc("/api/headroom/start", s.headroomStartAPI)
	mux.HandleFunc("/api/headroom/stop", s.headroomStopAPI)
	mux.HandleFunc("/api/headroom/restart", s.headroomRestartAPI)
	mux.HandleFunc("/api/headroom/proxy/", s.headroomProxyAPI)
	mux.HandleFunc("/api/headroom/extras", s.headroomExtrasAPI)
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

func (s *Server) modelsAvailabilityAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		now := time.Now().UnixMilli()
		models := []map[string]any{}
		for _, provider := range s.store.List() {
			for model, until := range provider.ModelLocks {
				if until > now {
					models = append(models, map[string]any{"provider": provider.ID, "model": model, "status": "cooldown", "until": until, "connectionId": provider.ID, "connectionName": provider.Name, "lastError": provider.LastError})
				}
			}
			if len(provider.ModelLocks) == 0 && provider.TestStatus == "unavailable" {
				models = append(models, map[string]any{"provider": provider.ID, "model": "__all", "status": "unavailable", "connectionId": provider.ID, "connectionName": provider.Name, "lastError": provider.LastError})
			}
		}
		writeJSON(w, 200, map[string]any{"models": models, "unavailableCount": len(models)})
		return
	}
	if r.Method == http.MethodPost {
		var input struct {
			Action   string `json:"action"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.Action != "clearCooldown" || input.Provider == "" || input.Model == "" {
			writeJSON(w, 400, map[string]string{"error": "Invalid request"})
			return
		}
		provider, ok := s.store.Find(input.Provider)
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "provider not found"})
			return
		}
		if provider.ModelLocks != nil {
			delete(provider.ModelLocks, input.Model)
		}
		if provider.TestStatus == "unavailable" {
			provider.TestStatus = "active"
			provider.LastError = ""
		}
		if err := s.store.Upsert(provider); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, 405, map[string]string{"error": "method not allowed"})
}

func (s *Server) cliToolsStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	tools := []string{"claude", "codex", "opencode", "droid", "openclaw", "hermes", "cowork", "copilot", "cline", "kilo", "deepseek-tui", "jcode", "grok-build"}
	result := map[string]any{}
	for _, tool := range tools {
		result[tool] = map[string]any{"installed": false, "configured": false, "available": false}
	}
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".claude", "settings.json")
		_, cliErr := exec.LookPath("claude")
		raw, readErr := os.ReadFile(path)
		configured := false
		if readErr == nil {
			var settings map[string]any
			_ = json.Unmarshal(raw, &settings)
			if env, ok := settings["env"].(map[string]any); ok {
				_, configured = env["ANTHROPIC_BASE_URL"]
			}
		}
		result["claude"] = map[string]any{"installed": cliErr == nil || readErr == nil, "configured": configured, "available": cliErr == nil || readErr == nil}
		codexPath := filepath.Join(home, ".codex", "config.toml")
		codexRaw, codexReadErr := os.ReadFile(codexPath)
		_, codexCLIErr := exec.LookPath("codex")
		codexInstalled := codexCLIErr == nil || codexReadErr == nil
		codexConfigured := strings.Contains(string(codexRaw), `[model_providers.9router]`) || strings.Contains(string(codexRaw), `model_provider = "9router"`)
		result["codex"] = map[string]any{"installed": codexInstalled, "configured": codexConfigured, "available": codexInstalled}
		opencodePath := filepath.Join(home, ".config", "opencode", "opencode.json")
		opencodeRaw, opencodeReadErr := os.ReadFile(opencodePath)
		_, opencodeCLIErr := exec.LookPath("opencode")
		opencodeInstalled := opencodeCLIErr == nil || opencodeReadErr == nil
		opencodeConfigured := strings.Contains(string(opencodeRaw), `"9router"`)
		result["opencode"] = map[string]any{"installed": opencodeInstalled, "configured": opencodeConfigured, "available": opencodeInstalled}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) claudeSettingsAPI(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	switch r.Method {
	case http.MethodGet:
		_, cliErr := exec.LookPath("claude")
		raw, readErr := os.ReadFile(settingsPath)
		settings := map[string]any(nil)
		if readErr == nil {
			_ = json.Unmarshal(raw, &settings)
		}
		installed := cliErr == nil || readErr == nil
		hasRouter := false
		if env, ok := settings["env"].(map[string]any); ok {
			_, hasRouter = env["ANTHROPIC_BASE_URL"]
		}
		writeJSON(w, http.StatusOK, map[string]any{"installed": installed, "settings": settings, "has9Router": hasRouter, "exaMcpEnabled": false, "settingsPath": settingsPath})
	case http.MethodPost:
		var input struct {
			Env map[string]any `json:"env"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.Env == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid env object"})
			return
		}
		if base, ok := input.Env["ANTHROPIC_BASE_URL"].(string); ok && base != "" && !strings.HasSuffix(base, "/v1") {
			input.Env["ANTHROPIC_BASE_URL"] = strings.TrimRight(base, "/") + "/v1"
		}
		current := map[string]any{}
		if raw, readErr := os.ReadFile(settingsPath); readErr == nil {
			_ = json.Unmarshal(raw, &current)
		}
		current["hasCompletedOnboarding"] = true
		env, _ := current["env"].(map[string]any)
		if env == nil {
			env = map[string]any{}
		}
		for key, value := range input.Env {
			env[key] = value
		}
		current["env"] = env
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0700); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		encoded, _ := json.MarshalIndent(current, "", "  ")
		if err := os.WriteFile(settingsPath, encoded, 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Settings updated successfully"})
	case http.MethodDelete:
		raw, readErr := os.ReadFile(settingsPath)
		if os.IsNotExist(readErr) {
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "No settings file to reset"})
			return
		}
		if readErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": readErr.Error()})
			return
		}
		current := map[string]any{}
		if json.Unmarshal(raw, &current) != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid settings JSON"})
			return
		}
		if env, ok := current["env"].(map[string]any); ok {
			for _, key := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_DEFAULT_OPUS_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_HAIKU_MODEL", "API_TIMEOUT_MS"} {
				delete(env, key)
			}
			if len(env) == 0 {
				delete(current, "env")
			}
		}
		encoded, _ := json.MarshalIndent(current, "", "  ")
		if err := os.WriteFile(settingsPath, encoded, 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Settings reset successfully"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) codexSettingsAPI(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	dir := filepath.Join(home, ".codex")
	configPath, authPath := filepath.Join(dir, "config.toml"), filepath.Join(dir, "auth.json")
	switch r.Method {
	case http.MethodGet:
		raw, readErr := os.ReadFile(configPath)
		_, cliErr := exec.LookPath("codex")
		installed := cliErr == nil || readErr == nil
		config := string(raw)
		writeJSON(w, 200, map[string]any{"installed": installed, "config": config, "has9Router": strings.Contains(config, `model_provider = "9router"`) || strings.Contains(config, "[model_providers.9router]"), "configPath": configPath})
	case http.MethodPost:
		var input struct {
			BaseURL       string `json:"baseUrl"`
			APIKey        string `json:"apiKey"`
			Model         string `json:"model"`
			SubagentModel string `json:"subagentModel"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.BaseURL == "" || input.APIKey == "" || input.Model == "" {
			writeJSON(w, 400, map[string]string{"error": "baseUrl, apiKey and model are required"})
			return
		}
		if !strings.HasSuffix(input.BaseURL, "/v1") {
			input.BaseURL = strings.TrimRight(input.BaseURL, "/") + "/v1"
		}
		raw, _ := os.ReadFile(configPath)
		config := string(raw)
		config = replaceTOMLLine(config, "model =", `model = "`+input.Model+`"`)
		config = replaceTOMLLine(config, "model_provider =", `model_provider = "9router"`)
		config = upsertTOMLSection(config, "[model_providers.9router]", []string{`name = "9Router"`, `base_url = "` + input.BaseURL + `"`, `wire_api = "responses"`})
		subagent := input.SubagentModel
		if subagent == "" {
			subagent = input.Model
		}
		config = upsertTOMLSection(config, "[agents.subagent]", []string{`model = "` + subagent + `"`})
		if err := os.MkdirAll(dir, 0700); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		auth := map[string]any{}
		if raw, readErr := os.ReadFile(authPath); readErr == nil {
			_ = json.Unmarshal(raw, &auth)
		}
		auth["OPENAI_API_KEY"], auth["auth_mode"] = input.APIKey, "apikey"
		encoded, _ := json.MarshalIndent(auth, "", "  ")
		_ = os.WriteFile(authPath, encoded, 0600)
		writeJSON(w, 200, map[string]any{"success": true, "message": "Codex settings applied successfully!", "configPath": configPath})
	case http.MethodDelete:
		raw, readErr := os.ReadFile(configPath)
		if os.IsNotExist(readErr) {
			writeJSON(w, 200, map[string]any{"success": true, "message": "No config file to reset"})
			return
		}
		if readErr != nil {
			writeJSON(w, 500, map[string]string{"error": readErr.Error()})
			return
		}
		config := string(raw)
		config = removeTOMLSection(config, "[model_providers.9router]")
		config = removeTOMLSection(config, "[agents.subagent]")
		config = removeTOMLLine(config, "model_provider = \"9router\"")
		if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if raw, readErr := os.ReadFile(authPath); readErr == nil {
			auth := map[string]any{}
			if json.Unmarshal(raw, &auth) == nil {
				delete(auth, "OPENAI_API_KEY")
				delete(auth, "auth_mode")
				if len(auth) == 0 {
					_ = os.Remove(authPath)
				} else if encoded, marshalErr := json.MarshalIndent(auth, "", "  "); marshalErr == nil {
					_ = os.WriteFile(authPath, encoded, 0600)
				}
			}
		}
		writeJSON(w, 200, map[string]any{"success": true, "message": "9Router settings removed successfully"})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) copilotSettingsAPI(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	configPath := filepath.Join(home, ".config", "Code", "User", "chatLanguageModels.json")
	if runtime.GOOS == "darwin" {
		configPath = filepath.Join(home, "Library", "Application Support", "Code", "User", "chatLanguageModels.json")
	}
	if runtime.GOOS == "windows" {
		configPath = filepath.Join(os.Getenv("APPDATA"), "Code", "User", "chatLanguageModels.json")
	}
	read := func() []any {
		raw, readErr := os.ReadFile(configPath)
		if readErr != nil {
			return []any{}
		}
		clean := regexp.MustCompile(`,\s*([}\]])`).ReplaceAll(raw, []byte(`$1`))
		var config []any
		if json.Unmarshal(clean, &config) != nil {
			return []any{}
		}
		return config
	}
	write := func(config []any) error {
		if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
			return err
		}
		raw, _ := json.MarshalIndent(config, "", "  ")
		return os.WriteFile(configPath, raw, 0600)
	}
	find := func(config []any) (int, map[string]any) {
		for index, raw := range config {
			if entry, ok := raw.(map[string]any); ok && entry["name"] == "9Router" {
				return index, entry
			}
		}
		return -1, nil
	}
	switch r.Method {
	case http.MethodGet:
		config := read()
		_, entry := find(config)
		response := map[string]any{"installed": true, "config": config, "has9Router": entry != nil, "configPath": configPath, "currentModel": nil, "currentUrl": nil}
		if entry != nil {
			if models, ok := entry["models"].([]any); ok && len(models) > 0 {
				if model, ok := models[0].(map[string]any); ok {
					response["currentModel"] = model["id"]
					response["currentUrl"] = model["url"]
				}
			}
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPost:
		var input struct {
			BaseURL string   `json:"baseUrl"`
			APIKey  string   `json:"apiKey"`
			Models  []string `json:"models"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.BaseURL == "" || len(input.Models) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "baseUrl and models are required"})
			return
		}
		models := make([]any, 0, len(input.Models))
		endpoint := strings.TrimRight(input.BaseURL, "/") + "/chat/completions#models.ai.azure.com"
		for _, id := range input.Models {
			models = append(models, map[string]any{"id": id, "name": id, "url": endpoint, "toolCalling": true, "vision": false, "maxInputTokens": 128000, "maxOutputTokens": 16000})
		}
		config := read()
		entry := map[string]any{"name": "9Router", "vendor": "azure", "apiKey": input.APIKey, "models": models}
		if entry["apiKey"] == "" {
			entry["apiKey"] = "sk_9router"
		}
		index, _ := find(config)
		if index >= 0 {
			config[index] = entry
		} else {
			config = append(config, entry)
		}
		if err := write(config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Copilot settings applied! Reload VS Code to take effect.", "configPath": configPath})
	case http.MethodDelete:
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "No config file to reset"})
			return
		}
		config := read()
		filtered := make([]any, 0, len(config))
		for _, raw := range config {
			if entry, ok := raw.(map[string]any); !ok || entry["name"] != "9Router" {
				filtered = append(filtered, raw)
			}
		}
		if err := write(filtered); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "9Router removed from Copilot config"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) droidSettingsAPI(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	dir, path := filepath.Join(home, ".factory"), filepath.Join(home, ".factory", "settings.json")
	read := func() map[string]any {
		raw, err := os.ReadFile(path)
		if err != nil {
			return map[string]any{}
		}
		raw = regexp.MustCompile(`,\s*([}\]])`).ReplaceAll(raw, []byte(`$1`))
		var settings map[string]any
		if json.Unmarshal(raw, &settings) != nil {
			return map[string]any{}
		}
		return settings
	}
	hasConfig := func(settings map[string]any) bool {
		models, _ := settings["customModels"].([]any)
		for _, raw := range models {
			if entry, ok := raw.(map[string]any); ok && strings.HasPrefix(anyString(entry["id"]), "custom:9Router") {
				return true
			}
		}
		return false
	}
	switch r.Method {
	case http.MethodGet:
		settings := read()
		_, cliErr := exec.LookPath("droid")
		_, fileErr := os.Stat(path)
		if cliErr != nil && fileErr != nil {
			writeJSON(w, 200, map[string]any{"installed": false, "settings": nil, "message": "Factory Droid CLI is not installed"})
			return
		}
		writeJSON(w, 200, map[string]any{"installed": true, "settings": settings, "has9Router": hasConfig(settings), "settingsPath": path})
	case http.MethodPost:
		var input struct {
			BaseURL     string   `json:"baseUrl"`
			APIKey      string   `json:"apiKey"`
			Model       string   `json:"model"`
			Models      []string `json:"models"`
			ActiveModel *string  `json:"activeModel"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		models := input.Models
		if len(models) == 0 && input.Model != "" {
			models = []string{input.Model}
		}
		if input.BaseURL == "" || len(models) == 0 {
			writeJSON(w, 400, map[string]string{"error": "baseUrl and at least one model are required"})
			return
		}
		settings := read()
		existing, _ := settings["customModels"].([]any)
		filtered := make([]any, 0, len(existing)+len(models))
		for _, raw := range existing {
			if entry, ok := raw.(map[string]any); !ok || !strings.HasPrefix(anyString(entry["id"]), "custom:9Router") {
				filtered = append(filtered, raw)
			}
		}
		baseURL := strings.TrimRight(input.BaseURL, "/")
		if !strings.HasSuffix(baseURL, "/v1") {
			baseURL += "/v1"
		}
		key := input.APIKey
		if key == "" {
			key = "your_api_key"
		}
		active := models[0]
		if input.ActiveModel != nil {
			active = *input.ActiveModel
		}
		for index, model := range models {
			if model == "" {
				continue
			}
			filtered = append(filtered, map[string]any{"model": model, "id": fmt.Sprintf("custom:9Router-%d", index), "index": index, "baseUrl": baseURL, "apiKey": key, "displayName": model, "maxOutputTokens": 131072, "noImageSupport": false, "provider": "openai"})
		}
		if active != "" {
			for index, raw := range filtered {
				entry, ok := raw.(map[string]any)
				if ok && entry["model"] == active {
					filtered = append([]any{entry}, append(filtered[:index], filtered[index+1:]...)...)
					break
				}
			}
			for index, raw := range filtered {
				if entry, ok := raw.(map[string]any); ok && strings.HasPrefix(anyString(entry["id"]), "custom:9Router") {
					entry["index"] = index
				}
			}
		}
		settings["customModels"] = filtered
		if err := os.MkdirAll(dir, 0700); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		raw, _ := json.MarshalIndent(settings, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "message": "Factory Droid settings applied successfully!", "settingsPath": path})
	case http.MethodDelete:
		if _, err := os.Stat(path); os.IsNotExist(err) {
			writeJSON(w, 200, map[string]any{"success": true, "message": "No settings file to reset"})
			return
		}
		settings := read()
		models, _ := settings["customModels"].([]any)
		filtered := make([]any, 0, len(models))
		for _, raw := range models {
			if entry, ok := raw.(map[string]any); !ok || !strings.HasPrefix(anyString(entry["id"]), "custom:9Router") {
				filtered = append(filtered, raw)
			}
		}
		if len(filtered) == 0 {
			delete(settings, "customModels")
		} else {
			settings["customModels"] = filtered
		}
		raw, _ := json.MarshalIndent(settings, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "message": "9Router settings removed successfully"})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func (s *Server) opencodeSettingsAPI(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	dir := filepath.Join(home, ".config", "opencode")
	path := filepath.Join(dir, "opencode.json")
	read := func() (map[string]any, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{}, nil
			}
			return nil, err
		}
		var config map[string]any
		if json.Unmarshal(raw, &config) != nil {
			return map[string]any{}, nil
		}
		return config, nil
	}
	write := func(config map[string]any) error {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		raw, _ := json.MarshalIndent(config, "", "  ")
		return os.WriteFile(path, raw, 0600)
	}
	switch r.Method {
	case http.MethodGet:
		config, _ := read()
		_, cliErr := exec.LookPath("opencode")
		_, fileErr := os.Stat(path)
		provider, _ := config["provider"].(map[string]any)
		entry, _ := provider["9router"].(map[string]any)
		models := []string{}
		if modelMap, ok := entry["models"].(map[string]any); ok {
			for id := range modelMap {
				models = append(models, id)
			}
		}
		active := ""
		if value, ok := config["model"].(string); ok && strings.HasPrefix(value, "9router/") {
			active = strings.TrimPrefix(value, "9router/")
		}
		writeJSON(w, 200, map[string]any{"installed": cliErr == nil || fileErr == nil, "config": config, "has9Router": entry != nil, "configPath": path, "opencode": map[string]any{"models": models, "activeModel": active}})
	case http.MethodPost:
		var input struct {
			BaseURL       string   `json:"baseUrl"`
			APIKey        string   `json:"apiKey"`
			Model         string   `json:"model"`
			Models        []string `json:"models"`
			ActiveModel   string   `json:"activeModel"`
			SubagentModel string   `json:"subagentModel"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		models := input.Models
		if len(models) == 0 && input.Model != "" {
			models = []string{input.Model}
		}
		if input.BaseURL == "" || len(models) == 0 {
			writeJSON(w, 400, map[string]string{"error": "baseUrl and at least one model are required"})
			return
		}
		if !strings.HasSuffix(input.BaseURL, "/v1") {
			input.BaseURL = strings.TrimRight(input.BaseURL, "/") + "/v1"
		}
		config, _ := read()
		provider, _ := config["provider"].(map[string]any)
		if provider == nil {
			provider = map[string]any{}
		}
		entry, _ := provider["9router"].(map[string]any)
		if entry == nil {
			entry = map[string]any{"npm": "@ai-sdk/openai-compatible"}
		}
		options, _ := entry["options"].(map[string]any)
		if options == nil {
			options = map[string]any{}
		}
		options["baseURL"] = input.BaseURL
		options["apiKey"] = input.APIKey
		if input.APIKey == "" {
			options["apiKey"] = "sk_9router"
		}
		entry["options"] = options
		modelMap, _ := entry["models"].(map[string]any)
		if modelMap == nil {
			modelMap = map[string]any{}
		}
		for _, model := range models {
			if model != "" {
				modelMap[model] = map[string]any{"name": model, "modalities": map[string]any{"input": []string{"text", "image"}, "output": []string{"text"}}}
			}
		}
		entry["models"] = modelMap
		provider["9router"] = entry
		config["provider"] = provider
		active := input.ActiveModel
		if active == "" {
			active = models[0]
		}
		if active != "" {
			config["model"] = "9router/" + active
		}
		agent, _ := config["agent"].(map[string]any)
		if agent == nil {
			agent = map[string]any{}
		}
		sub := input.SubagentModel
		if sub == "" {
			sub = models[0]
		}
		agent["explorer"] = map[string]any{"description": "Fast explorer subagent for codebase exploration", "mode": "subagent", "model": "9router/" + sub}
		config["agent"] = agent
		if err := write(config); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "message": "OpenCode settings applied successfully!", "configPath": path})
	case http.MethodPatch:
		var input struct {
			Clear bool `json:"clearActiveModel"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input)
		config, _ := read()
		if input.Clear {
			if model, ok := config["model"].(string); ok && strings.HasPrefix(model, "9router/") {
				config["model"] = ""
			}
		}
		if err := write(config); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "message": "Settings updated"})
	case http.MethodDelete:
		config, _ := read()
		provider, _ := config["provider"].(map[string]any)
		entry, _ := provider["9router"].(map[string]any)
		if model := r.URL.Query().Get("model"); model != "" && entry != nil {
			if models, ok := entry["models"].(map[string]any); ok {
				delete(models, model)
				if len(models) == 0 {
					delete(provider, "9router")
					delete(config, "model")
				} else if config["model"] == "9router/"+model {
					for id := range models {
						config["model"] = "9router/" + id
						break
					}
				}
			}
		} else {
			delete(provider, "9router")
			if model, ok := config["model"].(string); ok && strings.HasPrefix(model, "9router/") {
				delete(config, "model")
			}
		}
		if err := write(config); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "message": "9Router settings removed from OpenCode"})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) mcpAPI(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/mcp/"), "/"), "/")
	if len(parts) != 2 || (parts[1] != "sse" && parts[1] != "message") {
		http.NotFound(w, r)
		return
	}
	plugin := parts[0]
	envKey := "G9ROUTER_MCP_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(plugin)) + "_URL"
	target := strings.TrimSpace(os.Getenv(envKey))
	commandKey := "G9ROUTER_MCP_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(plugin)) + "_COMMAND"
	command := strings.TrimSpace(os.Getenv(commandKey))
	if target == "" && command == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Unknown plugin: " + plugin})
		return
	}
	if parts[1] == "message" {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if command != "" {
			if err := s.mcpBridge.Send(r.URL.Query().Get("sessionId"), json.RawMessage(body)); err != nil {
				writeJSON(w, 404, map[string]string{"error": err.Error()})
				return
			}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, strings.NewReader(string(body)))
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := s.client.Do(request)
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		defer response.Body.Close()
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if command != "" {
		sessionID, lines, err := s.mcpBridge.Start(r.Context(), command)
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = io.WriteString(w, "event: endpoint\ndata: /api/mcp/"+plugin+"/message?sessionId="+sessionID+"\n\n")
		flusher.Flush()
		for line := range lines {
			_, _ = io.WriteString(w, "data: "+line+"\n\n")
			flusher.Flush()
		}
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(response.StatusCode)
	if flusher, ok := w.(http.Flusher); ok {
		_, _ = io.WriteString(w, "event: endpoint\ndata: /api/mcp/"+plugin+"/message\n\n")
		flusher.Flush()
		streamCopy(w, response.Body, flusher)
	}
}

func (s *Server) headroomStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	urlValue := "http://127.0.0.1:8787"
	if configured, ok := s.settings.Get()["headroomUrl"].(string); ok && configured != "" {
		urlValue = configured
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(urlValue, "/")+"/health", nil)
	if err != nil {
		writeJSON(w, 200, map[string]any{"running": false, "url": urlValue, "error": err.Error()})
		return
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, 200, map[string]any{"running": false, "url": urlValue, "error": err.Error()})
		return
	}
	defer response.Body.Close()
	writeJSON(w, 200, map[string]any{"running": response.StatusCode < 400, "url": urlValue, "status": response.StatusCode})
}

func (s *Server) headroomStartAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	port := 8787
	if value, ok := s.settings.Get()["headroomPort"].(float64); ok && value > 0 && value < 65536 {
		port = int(value)
	}
	pid, err := s.headroomManager.Start(r.Context(), port)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error(), "code": "START_FAILED"})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "pid": pid, "port": port})
}
func (s *Server) headroomStopAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	stopped := s.headroomManager.Stop()
	if !stopped {
		writeJSON(w, 409, map[string]any{"stopped": false, "code": "NOT_RUNNING"})
		return
	}
	writeJSON(w, 200, map[string]any{"stopped": true})
}
func (s *Server) headroomRestartAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	_ = s.headroomManager.Stop()
	port := 8787
	pid, err := s.headroomManager.Start(r.Context(), port)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "pid": pid, "port": port})
}

func (s *Server) headroomProxyAPI(w http.ResponseWriter, r *http.Request) {
	base := "http://127.0.0.1:8787"
	if configured, ok := s.settings.Get()["headroomUrl"].(string); ok && configured != "" {
		base = strings.TrimRight(configured, "/")
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/api/headroom/proxy")
	target, err := url.Parse(base + suffix)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	target.RawQuery = r.URL.RawQuery
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), strings.NewReader(string(body)))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	request.Header = r.Header.Clone()
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		if key == "Content-Length" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (s *Server) headroomExtrasAPI(w http.ResponseWriter, r *http.Request) {
	allowed := map[string]bool{"code": true, "ml": true}
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		writeJSON(w, 200, map[string]any{"available": []string{"code", "ml"}, "python": false, "installed": map[string]bool{}})
		return
	}
	switch r.Method {
	case http.MethodGet:
		installed := map[string]bool{}
		output, _ := exec.Command(python, "-m", "pip", "list", "--format=json").Output()
		var packages []map[string]any
		if json.Unmarshal(output, &packages) == nil {
			for _, pkg := range packages {
				name, _ := pkg["name"].(string)
				if name == "tree-sitter" || name == "tree-sitter-language-pack" {
					installed["code"] = true
				}
				if name == "torch" || name == "huggingface-hub" {
					installed["ml"] = true
				}
			}
		}
		writeJSON(w, 200, map[string]any{"available": []string{"code", "ml"}, "python": true, "installed": installed})
	case http.MethodPost, http.MethodDelete:
		var input struct {
			Extras []string `json:"extras"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input)
		requested := []string{}
		for _, extra := range input.Extras {
			if allowed[extra] {
				requested = append(requested, extra)
			}
		}
		if len(requested) == 0 {
			writeJSON(w, 400, map[string]string{"error": "no valid extras"})
			return
		}
		args := []string{"-m", "pip"}
		if r.Method == http.MethodPost {
			spec := "headroom-ai[proxy," + strings.Join(requested, ",") + "]"
			args = append(args, "install", "--upgrade", spec)
		} else {
			packages := []string{}
			for _, extra := range requested {
				if extra == "code" {
					packages = append(packages, "tree-sitter", "tree-sitter-language-pack")
				}
				if extra == "ml" {
					packages = append(packages, "torch", "huggingface-hub")
				}
			}
			args = append(args, "uninstall", "-y")
			args = append(args, packages...)
		}
		command := exec.Command(python, args...)
		if output, err := command.CombinedOutput(); err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error(), "output": string(output)})
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "extras": requested})
	default:
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}
}

func replaceTOMLLine(content, prefix, replacement string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = replacement
			return strings.Join(lines, "\n")
		}
	}
	return strings.TrimRight(content, "\n") + "\n" + replacement + "\n"
}
func upsertTOMLSection(content, section string, lines []string) string {
	content = removeTOMLSection(content, section)
	return strings.TrimRight(content, "\n") + "\n\n" + section + "\n" + strings.Join(lines, "\n") + "\n"
}
func removeTOMLSection(content, section string) string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == section {
			start = i
			break
		}
	}
	if start < 0 {
		return content
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			end = i
			break
		}
	}
	return strings.Join(append(lines[:start], lines[end:]...), "\n")
}
func removeTOMLLine(content, target string) string {
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) != target {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
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
	var input map[string]any
	_ = json.Unmarshal(body, &input)
	model, _ := input["model"].(string)
	for _, provider := range s.store.Resolve(model) {
		if provider.OAuthID != "" {
			if credential, ok := s.oauth.Get(provider.OAuthID); ok {
				provider.APIKey = credential.AccessToken
			}
		}
		baseURL := strings.TrimSuffix(strings.TrimRight(provider.BaseURL, "/"), "/chat/completions")
		if baseURL != "" && s.proxy(w, r, baseURL, path, http.MethodPost, body, provider.APIKey) {
			return
		}
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
		if provider.OAuthID != "" {
			credential, ok := s.oauth.Get(provider.OAuthID)
			if ok && credential.ExpiringSoon(time.Now()) && credential.RefreshToken != "" {
				if refreshed, err := s.oauth.Refresh(r.Context(), provider.OAuthID); err == nil {
					credential = refreshed
				}
			}
			if ok {
				provider.APIKey = credential.AccessToken
			}
		}
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
		if provider.APIType == "vertex" {
			if s.proxyVertex(w, r, provider.BaseURL, model, request, provider.APIKey, provider.OAuthID != "", provider.ProviderSpecificData) {
				return
			}
			continue
		}
		if provider.APIType == "kiro" {
			if s.proxyKiro(w, r, provider.BaseURL, model, request, provider.APIKey) {
				return
			}
			continue
		}
		if provider.APIType == "cursor" {
			if s.proxyCursor(w, r, provider.BaseURL, model, request, provider.APIKey, provider.ProviderSpecificData) {
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

func cursorText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	parts, ok := value.([]any)
	if !ok {
		return ""
	}
	result := make([]string, 0, len(parts))
	for _, raw := range parts {
		part, _ := raw.(map[string]any)
		if text, ok := part["text"].(string); ok {
			result = append(result, text)
		}
	}
	return strings.Join(result, "")
}

func randomBytes(length int) []byte {
	result := make([]byte, length)
	if _, err := rand.Read(result); err != nil {
		return make([]byte, length)
	}
	return result
}

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

func (s *Server) proxyVertex(w http.ResponseWriter, incoming *http.Request, baseURL, model string, request map[string]any, apiKey string, oauthToken bool, specific map[string]any) bool {
	if !oauthToken && strings.HasPrefix(strings.TrimSpace(apiKey), "{") {
		account, err := vertex.ParseServiceAccount(apiKey)
		projectID := ""
		if err != nil {
			user, userErr := vertex.ParseAuthorizedUser(apiKey)
			if userErr != nil {
				return false
			}
			projectID = user.QuotaProject
		} else {
			projectID = account.ProjectID
		}
		token, err := vertex.AccessToken(incoming.Context(), s.client, apiKey)
		if err != nil {
			return false
		}
		apiKey, oauthToken = token, true
		if specific == nil {
			specific = map[string]any{}
		}
		if specific["projectId"] == nil {
			specific["projectId"] = projectID
		}
	}
	body, err := json.Marshal(translator.OpenAIToVertex(model, request))
	if err != nil {
		return false
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/publishers/google/models/" + model + ":generateContent"
	if oauthToken {
		project, _ := specific["projectId"].(string)
		location, _ := specific["location"].(string)
		if location == "" {
			location = "us-central1"
		}
		if project == "" {
			return false
		}
		endpoint = strings.TrimRight(baseURL, "/") + "/v1/projects/" + url.PathEscape(project) + "/locations/" + url.PathEscape(location) + "/publishers/google/models/" + url.PathEscape(model) + ":generateContent"
	} else if apiKey != "" {
		endpoint += "?key=" + url.QueryEscape(apiKey)
	}
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	if oauthToken && apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return false
	}
	var payload map[string]any
	if json.NewDecoder(response.Body).Decode(&payload) != nil {
		return false
	}
	writeJSON(w, response.StatusCode, translator.GeminiToOpenAI(model, payload))
	return true
}

func (s *Server) proxyKiro(w http.ResponseWriter, incoming *http.Request, baseURL, model string, request map[string]any, apiKey string) bool {
	body, err := json.Marshal(translator.OpenAIToKiro(model, request, true))
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	endpoint := strings.TrimRight(baseURL, "/") + "/generateAssistantResponse"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode >= 500 {
		return false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(response.StatusCode)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	state := &translator.KiroStreamState{}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	eventType := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, ":event-type:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, ":event-type:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		for _, output := range translator.KiroEventToOpenAISSE(eventType, []byte(payload), state) {
			_, _ = io.WriteString(w, output)
			flusher.Flush()
		}
	}
	return scanner.Err() == nil
}

func (s *Server) proxyCursor(w http.ResponseWriter, incoming *http.Request, baseURL, model string, request map[string]any, apiKey string, specific map[string]any) bool {
	messages := make([]map[string]string, 0)
	for _, raw := range arrayValue(request["messages"]) {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		content := cursorText(message["content"])
		if content != "" {
			messages = append(messages, map[string]string{"role": role, "content": content})
		}
	}
	model = strings.TrimPrefix(model, "cursor/")
	if model == "" {
		model = "default"
	}
	machineID, _ := specific["machineId"].(string)
	ghostMode := true
	if value, ok := specific["ghostMode"].(bool); ok {
		ghostMode = value
	}
	tools, _ := request["tools"].([]any)
	toolDefinitions := make([]map[string]any, 0, len(tools))
	for _, raw := range tools {
		if definition, ok := raw.(map[string]any); ok {
			toolDefinitions = append(toolDefinitions, definition)
		}
	}
	requestBody := cursor.Body(messages, model, ghostMode, toolDefinitions)
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/"), strings.NewReader(string(requestBody)))
	if err != nil {
		return false
	}
	for key, value := range cursor.Headers(apiKey, machineID, ghostMode) {
		upstreamRequest.Header.Set(key, value)
	}
	response, err := s.client.Do(upstreamRequest)
	if err != nil || response == nil {
		return false
	}
	if response.StatusCode >= 500 {
		response.Body.Close()
		return false
	}
	stream, _ := request["stream"].(bool)
	if !stream {
		defer response.Body.Close()
		payload, err := io.ReadAll(response.Body)
		if err != nil {
			return false
		}
		text := ""
		for len(payload) >= 5 {
			decoded, consumed, valid := cursor.ParseFrame(payload)
			if !valid {
				break
			}
			payload = payload[consumed:]
			text += decoded.Text
		}
		writeJSON(w, response.StatusCode, map[string]any{"id": "chatcmpl-" + hex.EncodeToString(randomBytes(12)), "object": "chat.completion", "created": time.Now().Unix(), "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop"}}})
		return response.StatusCode < 400
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(response.StatusCode)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return false
	}
	state := "chatcmpl-" + hex.EncodeToString(randomBytes(12))
	created := time.Now().Unix()
	all := make([]byte, 0)
	buffer := make([]byte, 32*1024)
	for {
		count, readErr := response.Body.Read(buffer)
		all = append(all, buffer[:count]...)
		for len(all) >= 5 {
			decoded, consumed, valid := cursor.ParseFrame(all)
			if !valid {
				break
			}
			all = all[consumed:]
			if decoded.Text == "" && decoded.Thinking == "" && decoded.ToolName == "" {
				continue
			}
			delta := map[string]any{}
			if decoded.Text != "" {
				delta["content"] = decoded.Text
			}
			if decoded.Thinking != "" {
				delta["reasoning_content"] = decoded.Thinking
			}
			if decoded.ToolName != "" {
				delta["tool_calls"] = []any{map[string]any{"index": 0, "id": decoded.ToolID, "type": "function", "function": map[string]any{"name": decoded.ToolName, "arguments": decoded.ToolArgs}}}
			}
			chunk := map[string]any{"id": state, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}}
			encoded, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
			flusher.Flush()
		}
		if readErr != nil {
			break
		}
	}
	_, _ = io.WriteString(w, fmt.Sprintf("data: %s\n\ndata: [DONE]\n\n", fmt.Sprintf(`{"id":%q,"object":"chat.completion.chunk","created":%d,"model":%q,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, state, created, model)))
	flusher.Flush()
	return response.StatusCode < 400
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
	if strings.Contains(baseURL, "chatgpt.com/backend-api/codex") {
		sum := sha256.Sum256(body)
		headers["originator"] = "codex_cli_rs"
		headers["session_id"] = hex.EncodeToString(sum[:16])
		headers["User-Agent"] = "codex_cli_rs/0.136.0"
	}
	if strings.Contains(baseURL, "api.githubcopilot.com") {
		headers["copilot-integration-id"] = "vscode-chat"
		headers["editor-version"] = "vscode/1.110.0"
		headers["editor-plugin-version"] = "copilot-chat/0.38.0"
		headers["openai-intent"] = "conversation-panel"
		headers["x-github-api-version"] = "2025-04-01"
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
