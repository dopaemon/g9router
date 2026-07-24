package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

const (
	kimchiWebURL        = "https://app.kimchi.dev"
	kimchiValidationURL = "https://api.cast.ai/v1/llm/openai/supported-providers"
	kimchiUserURL       = "https://app.kimchi.dev/api/v1/me"
)

func (s *Server) kimchiAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
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
	query := url.Values{"callback": {redirect}, "state": {state}}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": kimchiWebURL + "/cli-auth?" + query.Encode(), "state": state, "redirectUri": redirect, "provider": "kimchi"})
}

func (s *Server) kimchiExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}
	accessToken := strings.TrimSpace(input.Token)
	if accessToken == "" {
		accessToken = strings.TrimSpace(input.AccessToken)
	}
	if accessToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing Kimchi token"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, kimchiValidationURL, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Kimchi token validation failed"})
		return
	}
	user := map[string]any{}
	request, err = http.NewRequestWithContext(r.Context(), http.MethodGet, kimchiUserURL, nil)
	if err == nil {
		request.Header.Set("Accept", "application/json")
		request.Header.Set("Authorization", "Bearer "+accessToken)
		if userResponse, userErr := s.client.Do(request); userErr == nil {
			if userResponse.StatusCode >= 200 && userResponse.StatusCode < 300 {
				_ = json.NewDecoder(io.LimitReader(userResponse.Body, 1<<20)).Decode(&user)
			}
			userResponse.Body.Close()
		}
	}
	userID := kimchiString(user, "id")
	email := kimchiString(user, "email")
	if email == "" && userID != "" {
		email = "kimchi-user-" + userID
	}
	providerData := map[string]any{"authMethod": "browser_token", "userId": userID, "username": kimchiString(user, "username")}
	credentialID := "kimchi-oauth"
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "kimchi", AccessToken: accessToken, ExpiresAt: 0}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "kimchi", Name: "Kimchi", BaseURL: "https://llm.kimchi.dev/openai/v1/chat/completions", APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "email": email, "displayName": kimchiString(user, "name"), "providerSpecificData": providerData}, "connection": map[string]string{"provider": "kimchi", "id": credentialID}})
}

func kimchiString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
