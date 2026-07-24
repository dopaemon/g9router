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
	grokCLIDeviceURL = "https://auth.x.ai/oauth2/device/code"
	grokCLITokenURL  = "https://auth.x.ai/oauth2/token"
	grokCLIUserURL   = "https://cli-chat-proxy.grok.com/v1/user"
)

func (s *Server) grokCLIDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientID := os.Getenv("G9ROUTER_GROK_CLIENT_ID")
	if clientID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "G9ROUTER_GROK_CLIENT_ID is not configured"})
		return
	}
	form := url.Values{"client_id": {clientID}, "scope": {"openid profile email offline_access grok-cli:access api:access conversations:read conversations:write"}, "referrer": {"grok-build"}}
	payload, status, err := s.grokCLIRequest(r, http.MethodPost, grokCLIDeviceURL, form)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": "Grok CLI device code request failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) grokCLIPollAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientID := os.Getenv("G9ROUTER_GROK_CLIENT_ID")
	var input struct {
		DeviceCode string `json:"device_code"`
	}
	if clientID == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "G9ROUTER_GROK_CLIENT_ID is not configured"})
		return
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.DeviceCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing device_code"})
		return
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {input.DeviceCode}, "client_id": {clientID}}
	payload, _, err := s.grokCLIRequest(r, http.MethodPost, grokCLITokenURL, form)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "invalid_response", "errorDescription": err.Error()})
		return
	}
	accessToken := grokCLIString(payload, "access_token")
	if accessToken == "" {
		errorCode := grokCLIString(payload, "error")
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": errorCode, "errorDescription": grokCLIString(payload, "error_description"), "pending": errorCode == "authorization_pending"})
		return
	}
	profile := s.grokCLIProfile(r, accessToken)
	credentialID := "grok-cli-oauth"
	expiresIn := grokCLIInt64(payload, "expires_in")
	providerData := map[string]any{"authMethod": "device_code", "idToken": payload["id_token"], "email": profile["email"], "userId": profile["userId"], "hasGrokCodeAccess": profile["hasGrokCodeAccess"], "subscriptionTier": profile["subscriptionTier"]}
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "grok-cli", AccessToken: accessToken, RefreshToken: grokCLIString(payload, "refresh_token"), TokenURL: grokCLITokenURL, ClientID: clientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), Scope: grokCLIString(payload, "scope")}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "grok-cli", Name: "Grok CLI", BaseURL: "https://cli-chat-proxy.grok.com/v1/responses", APIKey: accessToken, APIType: "openai-responses", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": grokCLIString(payload, "refresh_token"), "expiresIn": expiresIn, "email": profile["email"], "displayName": profile["displayName"], "providerSpecificData": providerData}, "connection": map[string]string{"provider": "grok-cli", "id": credentialID}})
}

func (s *Server) grokCLIRequest(r *http.Request, method, endpoint string, form url.Values) (map[string]any, int, error) {
	request, err := http.NewRequestWithContext(r.Context(), method, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "grok-pager/0.2.93 grok-shell/0.2.93 (linux; x86_64)")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, http.StatusBadGateway, err
	}
	return payload, http.StatusOK, nil
}

func (s *Server) grokCLIProfile(r *http.Request, accessToken string) map[string]any {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, grokCLIUserURL, nil)
	if err != nil {
		return map[string]any{}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "grok-pager/0.2.93 grok-shell/0.2.93 (linux; x86_64)")
	request.Header.Set("x-xai-token-auth", "xai-grok-cli")
	request.Header.Set("x-grok-client-version", "0.2.93")
	response, err := s.client.Do(request)
	if err != nil {
		return map[string]any{}
	}
	defer response.Body.Close()
	var profile map[string]any
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile)
	}
	result := map[string]any{"email": profile["email"], "userId": profile["userId"], "hasGrokCodeAccess": profile["hasGrokCodeAccess"], "subscriptionTier": profile["subscriptionTier"]}
	first, _ := profile["firstName"].(string)
	last, _ := profile["lastName"].(string)
	result["displayName"] = strings.TrimSpace(first + " " + last)
	return result
}

func grokCLIString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func grokCLIInt64(payload map[string]any, key string) int64 {
	value, _ := payload[key].(float64)
	return int64(value)
}
