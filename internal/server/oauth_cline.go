package server

import (
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

const clineAuthorizeURL = "https://api.cline.bot/api/v1/auth/authorize"
const clineTokenURL = "https://api.cline.bot/api/v1/auth/token"

func (s *Server) clineAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	redirect := r.URL.Query().Get("redirect_uri")
	if redirect == "" {
		redirect = "http://localhost:8080/callback"
	}
	query := url.Values{"client_type": {"extension"}, "callback_url": {redirect}, "redirect_uri": {redirect}}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": clineAuthorizeURL + "?" + query.Encode(), "redirectUri": redirect, "provider": strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/authorize"), "/api/oauth/")})
}

func (s *Server) clineExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/exchange"), "/api/oauth/")
	if provider != "cline" && provider != "clinepass" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported Cline provider"})
		return
	}
	var input struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.Code == "" || input.RedirectURI == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		return
	}
	token := clineDecodeToken(input.Code)
	if token == nil {
		token = s.clineExchangeRemote(r, input.Code, input.RedirectURI)
	}
	if token == nil || clineString(token, "access_token") == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Cline token exchange failed"})
		return
	}
	accessToken := clineString(token, "access_token")
	expiresAt := clineExpiry(token)
	credentialID := provider + "-oauth"
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: provider, AccessToken: accessToken, RefreshToken: clineString(token, "refresh_token"), TokenURL: "https://api.cline.bot/api/v1/auth/refresh", ClientID: provider, ExpiresAt: expiresAt, Scope: ""}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	providerData := map[string]any{"firstName": token["firstName"], "lastName": token["lastName"]}
	if err := s.store.Upsert(providers.Provider{ID: provider, Name: provider, BaseURL: "https://api.cline.bot/api/v1/chat/completions", APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": clineString(token, "refresh_token"), "expiresAt": token["expires_at"], "email": token["email"], "providerSpecificData": providerData}, "connection": map[string]string{"provider": provider, "id": credentialID}})
}

func clineDecodeToken(code string) map[string]any {
	encoded := code
	if padding := len(encoded) % 4; padding != 0 {
		encoded += strings.Repeat("=", 4-padding)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(code, "="))
	}
	if err != nil {
		return nil
	}
	end := strings.LastIndex(string(data), "}")
	if end < 0 {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal(data[:end+1], &raw) != nil {
		return nil
	}
	return map[string]any{"access_token": raw["accessToken"], "refresh_token": raw["refreshToken"], "email": raw["email"], "firstName": raw["firstName"], "lastName": raw["lastName"], "expires_at": raw["expiresAt"]}
}

func (s *Server) clineExchangeRemote(r *http.Request, code, redirect string) map[string]any {
	body, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "code": code, "client_type": "extension", "redirect_uri": redirect})
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, clineTokenURL, strings.NewReader(string(body)))
	if err != nil {
		return nil
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}
	var payload map[string]any
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload) != nil {
		return nil
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		data = payload
	}
	return map[string]any{"access_token": data["accessToken"], "refresh_token": data["refreshToken"], "email": data["userInfo"], "expires_at": data["expiresAt"]}
}

func clineString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func clineExpiry(payload map[string]any) int64 {
	value := clineString(payload, "expires_at")
	if value == "" {
		return time.Now().Add(time.Hour).UnixMilli()
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UnixMilli()
	}
	return time.Now().Add(time.Hour).UnixMilli()
}
