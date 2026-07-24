package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

const (
	kiloCodeAPIBase   = "https://api.kilo.ai"
	kiloCodeDeviceURL = kiloCodeAPIBase + "/api/device-auth/codes"
)

func (s *Server) kiloCodeDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, kiloCodeDeviceURL, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many pending authorization requests. Please try again later."})
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Device auth initiation failed"})
		return
	}
	code := kiloCodeString(payload, "code")
	writeJSON(w, http.StatusOK, map[string]any{"device_code": code, "user_code": code, "verification_uri": payload["verificationUrl"], "verification_uri_complete": payload["verificationUrl"], "expires_in": kiloCodeInt64(payload, "expiresIn", 300), "interval": 3})
}

func (s *Server) kiloCodePollAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.DeviceCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing device_code"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, kiloCodeDeviceURL+"/"+input.DeviceCode, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "poll_failed", "errorDescription": err.Error()})
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusAccepted {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "authorization_pending", "pending": true})
		return
	}
	if response.StatusCode == http.StatusForbidden {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "access_denied", "errorDescription": "Authorization denied by user"})
		return
	}
	if response.StatusCode == http.StatusGone {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "expired_token", "errorDescription": "Authorization code expired"})
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || kiloCodeString(payload, "status") != "approved" || kiloCodeString(payload, "token") == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "authorization_pending", "pending": true})
		return
	}
	accessToken := kiloCodeString(payload, "token")
	profile := s.kiloCodeProfile(r, accessToken)
	credentialID := "kilocode-oauth"
	providerData := map[string]any{"orgId": profile["orgId"]}
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "kilocode", AccessToken: accessToken, TokenURL: "", ClientID: "kilocode", ExpiresAt: 0}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "kilocode", Name: "Kilo Code", BaseURL: kiloCodeAPIBase + "/api/openrouter/chat/completions", APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "email": payload["userEmail"], "providerSpecificData": providerData}, "connection": map[string]string{"provider": "kilocode", "id": credentialID}})
}

func (s *Server) kiloCodeProfile(r *http.Request, accessToken string) map[string]any {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, kiloCodeAPIBase+"/api/profile", nil)
	if err != nil {
		return map[string]any{}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := s.client.Do(request)
	if err != nil {
		return map[string]any{}
	}
	defer response.Body.Close()
	var payload map[string]any
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload)
	}
	if organizations, ok := payload["organizations"].([]any); ok && len(organizations) > 0 {
		if organization, ok := organizations[0].(map[string]any); ok {
			return map[string]any{"orgId": organization["id"]}
		}
	}
	return map[string]any{}
}

func kiloCodeString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func kiloCodeInt64(payload map[string]any, key string, fallback int64) int64 {
	value, ok := payload[key].(float64)
	if !ok || value <= 0 {
		return fallback
	}
	return int64(value)
}

var _ = strings.TrimSpace
var _ = time.Now
