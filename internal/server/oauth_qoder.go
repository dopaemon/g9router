package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

const (
	qoderDeviceTokenURL = "https://openapi.qoder.sh/api/v1/deviceToken/poll"
	qoderUserInfoURL    = "https://openapi.qoder.sh/api/v1/userinfo"
	qoderLoginURL       = "https://qoder.com/device/selectAccounts"
)

func (s *Server) qoderDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	verifier, err := randomURLToken(64)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	nonce, err := randomURLToken(16)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	machineID, err := randomURLToken(16)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	query := url.Values{"challenge": {challenge}, "challenge_method": {"S256"}, "machine_id": {machineID}, "nonce": {nonce}}
	writeJSON(w, http.StatusOK, map[string]any{"device_code": nonce, "user_code": strings.ToUpper(nonce[:8]), "verification_uri": qoderLoginURL, "verification_uri_complete": qoderLoginURL + "?" + query.Encode(), "expires_in": 300, "interval": 2, "codeVerifier": verifier, "_qoderNonce": nonce, "_qoderMachineId": machineID})
}

func (s *Server) qoderPollAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		DeviceCode   string `json:"device_code"`
		CodeVerifier string `json:"code_verifier"`
		MachineID    string `json:"_qoderMachineId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.DeviceCode == "" || input.CodeVerifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing device_code or code_verifier"})
		return
	}
	query := url.Values{"nonce": {input.DeviceCode}, "verifier": {input.CodeVerifier}, "challenge_method": {"S256"}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, qoderDeviceTokenURL+"?"+query.Encode(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Go-http-client/2.0")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "poll_failed", "errorDescription": err.Error()})
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusAccepted || response.StatusCode == http.StatusNotFound {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "authorization_pending", "pending": true})
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "poll_failed", "errorDescription": fmt.Sprintf("Poll failed: %d", response.StatusCode)})
		return
	}
	accessToken := qoderString(payload, "token")
	if accessToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "authorization_pending", "pending": true})
		return
	}
	profile := s.qoderProfile(r, accessToken)
	expiresAt := qoderExpiry(payload)
	expiresIn := int64(24 * time.Hour / time.Second)
	if remaining := (expiresAt - time.Now().UnixMilli()) / 1000; remaining > expiresIn {
		expiresIn = remaining
	}
	credentialID := "qoder-oauth"
	providerData := map[string]any{"authMethod": "device", "userId": qoderString(payload, "user_id"), "machineId": input.MachineID, "organizationId": profile["organizationId"]}
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "qoder", AccessToken: accessToken, RefreshToken: qoderString(payload, "refresh_token"), ExpiresAt: expiresAt}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "qoder", Name: "Qoder", BaseURL: "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation", APIKey: accessToken, APIType: "qoder", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": qoderString(payload, "refresh_token"), "expiresIn": expiresIn, "email": profile["email"], "displayName": profile["name"], "providerSpecificData": providerData}, "connection": map[string]string{"provider": "qoder", "id": credentialID}})
}

func (s *Server) qoderProfile(r *http.Request, accessToken string) map[string]any {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, qoderUserInfoURL, nil)
	if err != nil {
		return map[string]any{}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Go-http-client/2.0")
	response, err := s.client.Do(request)
	if err != nil {
		return map[string]any{}
	}
	defer response.Body.Close()
	var payload map[string]any
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload)
	}
	return map[string]any{"name": qoderString(payload, "name"), "email": qoderString(payload, "email"), "organizationId": qoderString(payload, "organization_id")}
}

func qoderExpiry(payload map[string]any) int64 {
	if value, ok := payload["expires_at"].(float64); ok && value > 0 {
		return int64(value)
	}
	if value := qoderString(payload, "expires_at"); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UnixMilli()
		}
	}
	if value, ok := payload["expires_in"].(float64); ok && value >= 0 {
		return time.Now().Add(time.Duration(value) * time.Second).UnixMilli()
	}
	return time.Now().Add(30 * 24 * time.Hour).UnixMilli()
}

func qoderString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
