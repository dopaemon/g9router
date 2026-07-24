package server

import (
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
	geminiAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiTokenURL     = "https://oauth2.googleapis.com/token"
	geminiUserInfoURL  = "https://www.googleapis.com/oauth2/v1/userinfo"
	geminiCloudCodeURL = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
)

func (s *Server) geminiAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
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
	query := url.Values{"client_id": {geminiClientID()}, "response_type": {"code"}, "redirect_uri": {redirect}, "scope": {"https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"}, "state": {state}, "access_type": {"offline"}, "prompt": {"consent"}}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": geminiAuthorizeURL + "?" + query.Encode(), "state": state, "redirectUri": redirect, "provider": "gemini-cli"})
}

func (s *Server) geminiExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
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
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {geminiClientID()}, "client_secret": {os.Getenv("G9ROUTER_GEMINI_CLIENT_SECRET")}, "code": {input.Code}, "redirect_uri": {input.RedirectURI}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, geminiTokenURL, strings.NewReader(form.Encode()))
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
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Token exchange failed"})
		return
	}
	accessToken := geminiValue(token, "access_token")
	if accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid token response"})
		return
	}
	expiresIn := geminiInt64(token, "expires_in")
	profile, projectID := s.geminiMetadata(r, accessToken)
	credentialID := "gemini-cli-oauth"
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "gemini-cli", AccessToken: accessToken, RefreshToken: geminiValue(token, "refresh_token"), TokenURL: geminiTokenURL, ClientID: geminiClientID(), ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), Scope: geminiValue(token, "scope")}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	providerData := map[string]any{"email": profile["email"], "projectId": projectID}
	if err := s.store.Upsert(providers.Provider{ID: "gemini-cli", Name: "Gemini CLI", BaseURL: "https://cloudcode-pa.googleapis.com/v1internal", APIKey: accessToken, APIType: "gemini-cli", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": geminiValue(token, "refresh_token"), "expiresIn": expiresIn, "email": profile["email"], "projectId": projectID}, "connection": map[string]string{"provider": "gemini-cli", "id": credentialID}})
}

func (s *Server) geminiMetadata(r *http.Request, accessToken string) (map[string]any, string) {
	profile := map[string]any{}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, geminiUserInfoURL+"?alt=json", nil)
	if err == nil {
		request.Header.Set("Authorization", "Bearer "+accessToken)
		if response, requestErr := s.client.Do(request); requestErr == nil {
			_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile)
			response.Body.Close()
		}
	}
	project := ""
	request, err = http.NewRequestWithContext(r.Context(), http.MethodPost, geminiCloudCodeURL, strings.NewReader(`{"metadata":{"ideType":9,"platform":1,"pluginType":2},"mode":1}`))
	if err == nil {
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("Content-Type", "application/json")
		if response, requestErr := s.client.Do(request); requestErr == nil {
			var payload map[string]any
			_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload)
			response.Body.Close()
			if value, ok := payload["cloudaicompanionProject"].(map[string]any); ok {
				project, _ = value["id"].(string)
			} else if value, ok := payload["cloudaicompanionProject"].(string); ok {
				project = value
			}
		}
	}
	return profile, project
}

func geminiValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func geminiInt64(payload map[string]any, key string) int64 {
	value, _ := payload[key].(float64)
	return int64(value)
}

func geminiClientID() string {
	return os.Getenv("G9ROUTER_GEMINI_CLIENT_ID")
}
