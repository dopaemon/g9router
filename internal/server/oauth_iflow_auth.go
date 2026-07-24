package server

import (
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

const (
	iflowAuthorizeURL = "https://iflow.cn/oauth"
	iflowTokenURL     = "https://iflow.cn/oauth/token"
	iflowUserURL      = "https://iflow.cn/api/oauth/getUserInfo"
)

func (s *Server) iflowAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientID := os.Getenv("G9ROUTER_IFLOW_CLIENT_ID")
	if clientID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "G9ROUTER_IFLOW_CLIENT_ID is not configured"})
		return
	}
	redirect := r.URL.Query().Get("redirect_uri")
	if redirect == "" {
		redirect = "http://localhost:8080/callback"
	}
	state, err := randomURLToken(24)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	query := url.Values{"loginMethod": {"phone"}, "type": {"phone"}, "redirect": {redirect}, "state": {state}, "client_id": {clientID}}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": iflowAuthorizeURL + "?" + query.Encode(), "state": state, "redirectUri": redirect, "provider": "iflow"})
}

func (s *Server) iflowExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientID, clientSecret := os.Getenv("G9ROUTER_IFLOW_CLIENT_ID"), os.Getenv("G9ROUTER_IFLOW_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "iFlow OAuth client credentials are not configured"})
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
	form := url.Values{"grant_type": {"authorization_code"}, "code": {input.Code}, "redirect_uri": {input.RedirectURI}, "client_id": {clientID}, "client_secret": {clientSecret}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, iflowTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	basic := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	request.Header.Set("Authorization", "Basic "+basic)
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
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "iFlow token exchange failed"})
		return
	}
	accessToken := iflowString(token, "access_token")
	if accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid token response"})
		return
	}
	user, err := s.iflowUser(r, accessToken)
	if err != nil || iflowString(user, "apiKey") == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "iFlow user info did not return an API key"})
		return
	}
	expiresIn := iflowInt64(token, "expires_in")
	credentialID := "iflow-oauth"
	providerData := map[string]any{"authType": "oauth", "email": user["email"], "phone": user["phone"]}
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "iflow", AccessToken: accessToken, RefreshToken: iflowString(token, "refresh_token"), TokenURL: iflowTokenURL, ClientID: clientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli()}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "iflow", Name: "iFlow", BaseURL: "https://apis.iflow.cn/v1/chat/completions", APIKey: iflowString(user, "apiKey"), APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": iflowString(token, "refresh_token"), "expiresIn": expiresIn, "apiKey": iflowString(user, "apiKey"), "email": user["email"]}, "connection": map[string]string{"provider": "iflow", "id": credentialID}})
}

func (s *Server) iflowUser(r *http.Request, accessToken string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, iflowUserURL+"?accessToken="+url.QueryEscape(accessToken), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var wrapper struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&wrapper); err != nil {
		return nil, err
	}
	if !wrapper.Success {
		return nil, os.ErrPermission
	}
	return wrapper.Data, nil
}

func iflowString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func iflowInt64(payload map[string]any, key string) int64 {
	value, _ := payload[key].(float64)
	return int64(value)
}
