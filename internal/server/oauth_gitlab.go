package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

func (s *Server) gitlabPATAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Token   string `json:"token"`
		BaseURL string `json:"baseUrl"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Personal Access Token is required"})
		return
	}
	base := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if base == "" {
		base = "https://gitlab.com"
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base+"/api/v4/user", nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Private-Token", strings.TrimSpace(input.Token))
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := http.StatusUnauthorized
		if response.StatusCode >= 500 {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]string{"error": "GitLab token verification failed"})
		return
	}
	var user struct {
		Username   string `json:"username"`
		Name       string `json:"name"`
		Email      string `json:"email"`
		PublicMail string `json:"public_email"`
	}
	if json.Unmarshal(data, &user) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid GitLab user response"})
		return
	}
	email := user.Email
	if email == "" {
		email = user.PublicMail
	}
	if err := s.store.Upsert(providers.Provider{ID: "gitlab", Name: "GitLab Duo", BaseURL: base + "/v1", APIKey: strings.TrimSpace(input.Token), APIType: "openai", Enabled: true, TestStatus: "active", ProviderSpecificData: map[string]any{"username": usernameOr(user.Username, email), "email": email, "name": user.Name, "baseUrl": base, "authKind": "personal_access_token"}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "gitlab", "email": email})
}

func usernameOr(username, fallback string) string {
	if username != "" {
		return username
	}
	return fallback
}

func (s *Server) gitlabAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id is required"})
		return
	}
	baseURL := strings.TrimRight(r.URL.Query().Get("base_url"), "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	redirect := r.URL.Query().Get("redirect_uri")
	if redirect == "" {
		redirect = "http://localhost:8080/callback"
	}
	verifier, err := randomURLToken(64)
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
	query := url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "state": {state}, "scope": {"api read_user"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": baseURL + "/oauth/authorize?" + query.Encode(), "state": state, "codeVerifier": verifier, "codeChallenge": challenge, "redirectUri": redirect, "baseUrl": baseURL, "provider": "gitlab"})
}

func (s *Server) gitlabExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Code         string `json:"code"`
		RedirectURI  string `json:"redirect_uri"`
		CodeVerifier string `json:"code_verifier"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		BaseURL      string `json:"base_url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.Code == "" || input.RedirectURI == "" || input.CodeVerifier == "" || input.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		return
	}
	baseURL := strings.TrimRight(input.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://gitlab.com"
	}
	form := url.Values{"client_id": {input.ClientID}, "grant_type": {"authorization_code"}, "code": {input.Code}, "redirect_uri": {input.RedirectURI}, "code_verifier": {input.CodeVerifier}}
	if input.ClientSecret != "" {
		form.Set("client_secret", input.ClientSecret)
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, baseURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	var token map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "GitLab token exchange failed"})
		return
	}
	accessToken := gitlabString(token, "access_token")
	if accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid token response"})
		return
	}
	user := s.gitlabUser(r, baseURL, accessToken)
	credentialID := "gitlab-oauth"
	expiresIn := gitlabInt64(token, "expires_in")
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "gitlab", AccessToken: accessToken, RefreshToken: gitlabString(token, "refresh_token"), TokenURL: baseURL + "/oauth/token", ClientID: input.ClientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), Scope: gitlabString(token, "scope")}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	providerData := map[string]any{"username": user["username"], "email": user["email"], "name": user["name"], "baseUrl": baseURL, "clientId": input.ClientID, "authKind": "oauth"}
	if err := s.store.Upsert(providers.Provider{ID: "gitlab", Name: "GitLab Duo", BaseURL: baseURL + "/api/v4/chat/completions", APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": gitlabString(token, "refresh_token"), "expiresIn": expiresIn, "providerSpecificData": providerData}, "connection": map[string]string{"provider": "gitlab", "id": credentialID}})
}

func (s *Server) gitlabUser(r *http.Request, baseURL, accessToken string) map[string]any {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, baseURL+"/api/v4/user", nil)
	if err != nil {
		return map[string]any{}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := s.client.Do(request)
	if err != nil {
		return map[string]any{}
	}
	defer response.Body.Close()
	var user map[string]any
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&user)
	}
	return user
}

func gitlabString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func gitlabInt64(payload map[string]any, key string) int64 {
	value, _ := payload[key].(float64)
	return int64(value)
}
