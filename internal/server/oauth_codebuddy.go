package server

import (
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
	codeBuddyStateURL = "https://copilot.tencent.com/v2/plugin/auth/state"
	codeBuddyTokenURL = "https://copilot.tencent.com/v2/plugin/auth/token"
	codeBuddyBaseURL  = "https://copilot.tencent.com/v2/chat/completions"
)

func (s *Server) codeBuddyDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, codeBuddyStateURL+"?platform=CLI", strings.NewReader("{}"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	codeBuddyHeaders(request)
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "CodeBuddy state request failed"})
		return
	}
	data, _ := payload["data"].(map[string]any)
	state, _ := data["state"].(string)
	authURL, _ := data["authUrl"].(string)
	if payload["code"] != float64(0) || state == "" || authURL == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "CodeBuddy state error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_code": state, "user_code": "", "verification_uri": authURL, "interval": 5})
}

func (s *Server) codeBuddyPollAPI(w http.ResponseWriter, r *http.Request) {
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
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, codeBuddyTokenURL+"?"+url.Values{"state": {input.DeviceCode}}.Encode(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	codeBuddyHeaders(request)
	request.Header.Set("X-No-Enterprise-Id", "true")
	request.Header.Set("X-No-Department-Info", "true")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "request_failed"})
		return
	}
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "request_failed"})
		return
	}
	code, _ := payload["code"].(float64)
	data, _ := payload["data"].(map[string]any)
	accessToken, _ := data["accessToken"].(string)
	if code == 11217 {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": "authorization_pending", "pending": true})
		return
	}
	if code != 0 || accessToken == "" {
		message, _ := payload["msg"].(string)
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": message})
		return
	}
	credentialID := "codebuddy-cn-oauth"
	expiresIn := int64(86400)
	if value, ok := data["expiresIn"].(float64); ok && value > 0 {
		expiresIn = int64(value)
	}
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "codebuddy-cn", AccessToken: accessToken, RefreshToken: codeBuddyString(data, "refreshToken"), ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli()}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "codebuddy-cn", Name: "CodeBuddy CN", BaseURL: codeBuddyBaseURL, APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active"}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": codeBuddyString(data, "refreshToken"), "expiresIn": expiresIn}, "connection": map[string]string{"provider": "codebuddy-cn", "id": credentialID}})
}

func codeBuddyHeaders(request *http.Request) {
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CLI/2.63.2 CodeBuddy/2.63.2")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	request.Header.Set("X-Domain", "copilot.tencent.com")
	request.Header.Set("X-No-Authorization", "true")
	request.Header.Set("X-No-User-Id", "true")
	request.Header.Set("X-Product", "SaaS")
}

func codeBuddyString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
