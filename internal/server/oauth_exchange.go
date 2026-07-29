package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

func (s *Server) genericOAuthExchangeAPI(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/oauth/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "exchange" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported OAuth action"})
		return
	}
	provider := parts[0]
	var input struct {
		Code         string `json:"code"`
		RedirectURI  string `json:"redirect_uri"`
		CodeVerifier string `json:"code_verifier"`
		State        string `json:"state"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.Code == "" || input.RedirectURI == "" || input.CodeVerifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		return
	}
	clientID, tokenURL := "", ""
	if provider == "claude" {
		clientID, tokenURL = "9d1c250a-e61b-44d9-88ed-5944d1962f5e", "https://api.anthropic.com/v1/oauth/token"
	} else if provider == "codex" {
		clientID, tokenURL = "app_EMoamEEZ73f0CkXaXp7hrann", "https://auth.openai.com/oauth/token"
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported OAuth provider"})
		return
	}
	var body io.Reader
	if provider == "claude" {
		data, _ := json.Marshal(map[string]string{"code": input.Code, "state": input.State, "grant_type": "authorization_code", "client_id": clientID, "redirect_uri": input.RedirectURI, "code_verifier": input.CodeVerifier})
		body = strings.NewReader(string(data))
	} else {
		values := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {input.Code}, "redirect_uri": {input.RedirectURI}, "code_verifier": {input.CodeVerifier}}
		body = strings.NewReader(values.Encode())
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", map[bool]string{true: "application/json", false: "application/x-www-form-urlencoded"}[provider == "claude"])
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Token exchange failed"})
		return
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if json.Unmarshal(data, &token) != nil || token.AccessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid token response"})
		return
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	credentialID := provider + "-oauth"
	var codexAccountValue providers.Account
	if provider == "codex" {
		codexAccountValue = codexAccountFromTokens(token.AccessToken, token.IDToken, "")
		credentialID = codexAccountValue.ID + "-oauth"
	}
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: provider, AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenURL: tokenURL, ClientID: clientID, ExpiresAt: time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli(), Scope: token.Scope}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	baseURL, apiType := "https://api.anthropic.com/v1/messages", "anthropic"
	name, email := provider, ""
	if provider == "codex" {
		baseURL, apiType = "https://chatgpt.com/backend-api/codex", "openai-responses"
		email = codexAccountValue.Email
		if email != "" {
			name = "Codex " + email
		}
	}
	connection := providers.Provider{ID: provider, Name: name, BaseURL: baseURL, APIKey: token.AccessToken, APIType: apiType, OAuthID: credentialID, Enabled: true, TestStatus: "active"}
	if provider == "codex" {
		existing, _ := s.store.Find(provider)
		connection = existing
		connection.ID, connection.Name, connection.BaseURL, connection.APIType = provider, name, baseURL, apiType
		connection.Enabled, connection.TestStatus, connection.OAuthID, connection.APIKey = true, "active", credentialID, token.AccessToken
		if len(connection.Accounts) == 0 && existing.APIKey != "" {
			connection.Accounts = append(connection.Accounts, providers.Account{ID: "codex-legacy", APIKey: existing.APIKey, OAuthID: existing.OAuthID, Name: existing.Name, Enabled: existing.Enabled})
		}
		codexAccountValue.OAuthID = credentialID
		updated := false
		for index := range connection.Accounts {
			if codexAccountID(connection.Accounts[index].Plan) == codexAccountID(codexAccountValue.Plan) {
				connection.Accounts[index] = codexAccountValue
				updated = true
				break
			}
		}
		if !updated {
			connection.Accounts = append(connection.Accounts, codexAccountValue)
		}
	}
	if err := s.store.Upsert(connection); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	connectionID := credentialID
	if provider == "codex" {
		connectionID = codexAccountValue.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "connection": map[string]any{"provider": provider, "id": connectionID, "email": email, "name": name, "plan": codexAccountValue.Plan}})
}
