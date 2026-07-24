package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

const xaiTokenURL = "https://auth.x.ai/oauth2/token"

func (s *Server) xaiAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientID := strings.TrimSpace(os.Getenv("G9ROUTER_XAI_CLIENT_ID"))
	if clientID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "xAI OAuth client ID is not configured"})
		return
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
	redirect := "http://127.0.0.1:56121/callback"
	query := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect},
		"scope":          {"openid profile email offline_access grok-cli:access api:access"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"}, "state": {state},
		"nonce": {randomNonce()}, "plan": {"generic"}, "referrer": {"cli-proxy-api"},
	}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": "https://auth.x.ai/oauth2/authorize?" + query.Encode(), "state": state, "codeVerifier": verifier, "codeChallenge": challenge, "redirectUri": redirect, "provider": "xai"})
}

func randomNonce() string {
	nonce, err := randomURLToken(24)
	if err != nil {
		return "g9router"
	}
	return nonce
}

func (s *Server) xaiExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct{ Code, RedirectURI, CodeVerifier string }
	var raw struct {
		Code         string `json:"code"`
		RedirectURI  string `json:"redirect_uri"`
		CodeVerifier string `json:"code_verifier"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&raw) != nil || raw.Code == "" || raw.RedirectURI == "" || raw.CodeVerifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		return
	}
	input.Code, input.RedirectURI, input.CodeVerifier = raw.Code, raw.RedirectURI, raw.CodeVerifier
	clientID := strings.TrimSpace(os.Getenv("G9ROUTER_XAI_CLIENT_ID"))
	if clientID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "xAI OAuth client ID is not configured"})
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {input.Code}, "redirect_uri": {input.RedirectURI}, "code_verifier": {input.CodeVerifier}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, xaiTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
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
	credentialID := "xai-oauth"
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "xai", AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, TokenURL: xaiTokenURL, ClientID: clientID, ExpiresAt: time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli(), Scope: token.Scope}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	providerData := map[string]any{"authMethod": "oauth", "scope": token.Scope}
	if token.IDToken != "" {
		providerData["idToken"] = token.IDToken
	}
	if claims := cursorTokenClaims(token.IDToken); claims != nil {
		if email, ok := claims["email"].(string); ok {
			providerData["email"] = email
		}
	}
	if err := s.store.Upsert(providers.Provider{ID: "xai", Name: "xAI", BaseURL: "https://api.x.ai/v1/chat/completions", APIKey: token.AccessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": token.AccessToken, "refreshToken": token.RefreshToken, "expiresIn": token.ExpiresIn, "providerSpecificData": providerData}, "connection": map[string]string{"provider": "xai", "id": credentialID}})
}
