package server

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) genericOAuthAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.genericOAuthExchangeAPI(w, r)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/oauth/"), "/"), "/")
	if len(parts) == 2 && (parts[1] == "start-proxy" || parts[1] == "poll-status" || parts[1] == "stop-proxy") {
		s.oauthProxyAPI(w, r)
		return
	}
	if len(parts) != 2 || parts[1] != "authorize" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported OAuth action"})
		return
	}
	provider := parts[0]
	redirect := r.URL.Query().Get("redirect_uri")
	if redirect == "" {
		redirect = "http://localhost:8080/callback"
	}
	verifier, err := randomURLToken(32)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	state, err := randomURLToken(24)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	config := map[string]struct {
		clientID, authURL, scope string
		extra                    map[string]string
	}{
		"claude": {"9d1c250a-e61b-44d9-88ed-5944d1962f5e", "https://claude.ai/oauth/authorize", "org:create_api_key user:profile user:inference", nil},
		"codex":  {"app_EMoamEEZ73f0CkXaXp7hrann", "https://auth.openai.com/oauth/authorize", "openid profile email offline_access", map[string]string{"id_token_add_organizations": "true", "codex_cli_simplified_flow": "true", "originator": "codex_cli_rs"}},
	}
	entry, ok := config[provider]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported OAuth provider"})
		return
	}
	query := url.Values{"code": {"true"}, "client_id": {entry.clientID}, "response_type": {"code"}, "redirect_uri": {redirect}, "scope": {entry.scope}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {state}}
	for key, value := range entry.extra {
		query.Set(key, value)
	}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": entry.authURL + "?" + query.Encode(), "state": state, "codeVerifier": verifier, "codeChallenge": challenge, "provider": provider})
}
