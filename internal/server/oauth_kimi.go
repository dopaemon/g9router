package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

const (
	kimiClientID  = "17e5f671-d194-4dfb-9706-5516cb48c098"
	kimiDeviceURL = "https://auth.kimi.com/api/oauth/device_authorization"
	kimiTokenURL  = "https://auth.kimi.com/api/oauth/token"
)

func (s *Server) kimiDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	deviceID, err := randomURLToken(16)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	payload, status, err := s.kimiRequest(r, http.MethodPost, kimiDeviceURL, url.Values{"client_id": {kimiClientID}}, deviceID)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if payload["verification_uri"] == nil {
		payload["verification_uri"] = "https://www.kimi.com/code/authorize_device"
	}
	if payload["verification_uri_complete"] == nil {
		if code := kimiString(payload, "user_code"); code != "" {
			payload["verification_uri_complete"] = "https://www.kimi.com/code/authorize_device?user_code=" + url.QueryEscape(code)
		}
	}
	payload["_kimiDeviceId"] = deviceID
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) kimiPollAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		DeviceCode   string `json:"device_code"`
		KimiDeviceID string `json:"_kimiDeviceId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.DeviceCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing device_code"})
		return
	}
	payload, _, err := s.kimiRequest(r, http.MethodPost, kimiTokenURL, url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "client_id": {kimiClientID}, "device_code": {input.DeviceCode}}, input.KimiDeviceID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "poll_failed", "errorDescription": err.Error()})
		return
	}
	if accessToken := kimiString(payload, "access_token"); accessToken != "" {
		credentialID := "kimi-oauth"
		expiresIn := kimiInt64(payload, "expires_in")
		if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "kimi", AccessToken: accessToken, RefreshToken: kimiString(payload, "refresh_token"), TokenURL: kimiTokenURL, ClientID: kimiClientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), Scope: kimiString(payload, "scope")}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		providerData := map[string]any{"authMethod": "device_code", "deviceId": input.KimiDeviceID}
		if err := s.store.Upsert(providers.Provider{ID: "kimi", Name: "Kimi", BaseURL: "https://api.kimi.com/coding/v1/chat/completions", APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": kimiString(payload, "refresh_token"), "expiresIn": expiresIn, "providerSpecificData": providerData}, "connection": map[string]string{"provider": "kimi", "id": credentialID}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": kimiString(payload, "error"), "errorDescription": kimiString(payload, "error_description"), "pending": kimiString(payload, "error") == "authorization_pending"})
}

func (s *Server) kimiRequest(r *http.Request, method, endpoint string, form url.Values, deviceID string) (map[string]any, int, error) {
	request, err := http.NewRequestWithContext(r.Context(), method, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Msh-Platform", "9router")
	request.Header.Set("X-Msh-Version", "0.1.0")
	request.Header.Set("X-Msh-Device-Name", hostname())
	request.Header.Set("X-Msh-Device-Model", runtime.GOOS+" "+runtime.GOARCH)
	request.Header.Set("X-Msh-Device-Id", deviceID)
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

func hostname() string {
	value, err := os.Hostname()
	if err != nil || value == "" {
		return "unknown"
	}
	return value
}

func kimiString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func kimiInt64(payload map[string]any, key string) int64 {
	value, _ := payload[key].(float64)
	return int64(value)
}
