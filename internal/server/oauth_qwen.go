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

const (
	qwenClientID  = "f0304373b74a44d2b584a3fb70ca9e56"
	qwenDeviceURL = "https://chat.qwen.ai/api/v1/oauth2/device/code"
	qwenTokenURL  = "https://chat.qwen.ai/api/v1/oauth2/token"
)

func (s *Server) qwenDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	verifier, err := randomURLToken(64)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	form := url.Values{"client_id": {qwenClientID}, "scope": {"openid profile email model.completion"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	payload, status, err := s.qwenRequestJSON(r, http.MethodPost, qwenDeviceURL, form)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	payload["codeVerifier"] = verifier
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) qwenPollAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		DeviceCode   string `json:"device_code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.DeviceCode == "" || input.CodeVerifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing device_code or code_verifier"})
		return
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "client_id": {qwenClientID}, "device_code": {input.DeviceCode}, "code_verifier": {input.CodeVerifier}}
	payload, _, err := s.qwenRequestJSON(r, http.MethodPost, qwenTokenURL, form)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": payload["error"], "errorDescription": firstString(payload, "error_description", "message")})
		return
	}
	accessToken := qwenStringValue(payload, "access_token")
	if accessToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "no_access_token", "errorDescription": "No access token received"})
		return
	}
	expiresIn := int64Value(payload, "expires_in")
	credentialID := "qwen-oauth"
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "qwen", AccessToken: accessToken, RefreshToken: qwenStringValue(payload, "refresh_token"), TokenURL: qwenTokenURL, ClientID: qwenClientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), Scope: qwenStringValue(payload, "scope")}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	providerData := map[string]any{"resourceUrl": payload["resource_url"]}
	if err := s.store.Upsert(providers.Provider{ID: "qwen", Name: "Qwen", BaseURL: "https://portal.qwen.ai/v1/chat/completions", APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": qwenStringValue(payload, "refresh_token"), "expiresIn": expiresIn, "providerSpecificData": providerData}, "connection": map[string]string{"provider": "qwen", "id": credentialID}})
}

func (s *Server) qwenRequestJSON(r *http.Request, method, endpoint string, form url.Values) (map[string]any, int, error) {
	request, err := http.NewRequestWithContext(r.Context(), method, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, http.StatusBadGateway, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return payload, http.StatusOK, nil
	}
	return payload, http.StatusOK, nil
}

func int64Value(payload map[string]any, key string) int64 {
	value, _ := payload[key].(float64)
	return int64(value)
}

func qwenStringValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
