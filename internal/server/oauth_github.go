package server

import (
	"context"
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
	githubClientID        = "Iv1.b507a08c87ecfe98"
	githubDeviceCodeURL   = "https://github.com/login/device/code"
	githubTokenURL        = "https://github.com/login/oauth/access_token"
	githubUserInfoURL     = "https://api.github.com/user"
	githubCopilotTokenURL = "https://api.github.com/copilot_internal/v2/token"
)

func (s *Server) githubDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	form := url.Values{"client_id": {githubClientID}, "scope": {"read:user"}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, githubDeviceCodeURL, strings.NewReader(form.Encode()))
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
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("Device code request failed: %s", strings.TrimSpace(string(data)))})
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid device code response"})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) githubPollAPI(w http.ResponseWriter, r *http.Request) {
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
	form := url.Values{"client_id": {githubClientID}, "device_code": {input.DeviceCode}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, githubTokenURL, strings.NewReader(form.Encode()))
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
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		payload = map[string]any{"error": "invalid_response", "error_description": strings.TrimSpace(string(data))}
	}
	accessToken, _ := payload["access_token"].(string)
	if accessToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": payload["error"], "errorDescription": firstString(payload, "error_description", "message")})
		return
	}
	extra := s.githubExchangeMetadata(r, accessToken)
	credentialID := "github-oauth"
	expiresIn := int64(0)
	if value, ok := payload["expires_in"].(float64); ok && value > 0 {
		expiresIn = int64(value)
	}
	credential := oauth.Credential{ID: credentialID, Provider: "github", AccessToken: accessToken, RefreshToken: githubStringValue(payload, "refresh_token"), TokenURL: githubTokenURL, ClientID: githubClientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), Scope: githubStringValue(payload, "scope")}
	if err := s.oauth.Upsert(credential); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	providerData := map[string]any{"copilotToken": extra["copilotToken"], "copilotTokenExpiresAt": extra["copilotTokenExpiresAt"], "githubUserId": extra["githubUserId"], "githubLogin": extra["githubLogin"], "githubName": extra["githubName"], "githubEmail": extra["githubEmail"]}
	if err := s.store.Upsert(providers.Provider{ID: "github", Name: "GitHub Copilot", BaseURL: "https://api.githubcopilot.com", APIKey: accessToken, APIType: "openai", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "name": extra["githubLogin"], "displayName": extra["githubName"], "email": extra["githubEmail"], "providerSpecificData": providerData}, "connection": map[string]string{"provider": "github", "id": credentialID}})
}

func (s *Server) githubExchangeMetadata(r *http.Request, accessToken string) map[string]any {
	result := map[string]any{}
	for key, endpoint := range map[string]string{"copilot": githubCopilotTokenURL, "user": githubUserInfoURL} {
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
		if err != nil {
			continue
		}
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		request.Header.Set("User-Agent", "GitHubCopilotChat/0.26.7")
		response, err := s.client.Do(request)
		if err != nil {
			continue
		}
		var payload map[string]any
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload)
		response.Body.Close()
		if key == "copilot" {
			result["copilotToken"] = payload["token"]
			result["copilotTokenExpiresAt"] = payload["expires_at"]
		} else {
			result["githubUserId"] = payload["id"]
			result["githubLogin"] = payload["login"]
			result["githubName"] = payload["name"]
			result["githubEmail"] = payload["email"]
		}
	}
	return result
}

func githubStringValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func (s *Server) refreshGithubCopilotToken(ctx context.Context, accessToken string) (string, any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubCopilotTokenURL, nil)
	if err != nil {
		return "", nil, err
	}
	request.Header.Set("Authorization", "token "+accessToken)
	request.Header.Set("User-Agent", "GitHubCopilotChat/0.26.7")
	request.Header.Set("Editor-Version", "vscode/1.110.0")
	request.Header.Set("Editor-Plugin-Version", "copilot-chat/0.38.0")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-github-api-version", "2025-04-01")
	response, err := s.client.Do(request)
	if err != nil {
		return "", nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", nil, fmt.Errorf("copilot token status %s", response.Status)
	}
	var payload struct {
		Token     string `json:"token"`
		ExpiresAt any    `json:"expires_at"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return "", nil, err
	}
	if payload.Token == "" {
		return "", nil, fmt.Errorf("copilot response missing token")
	}
	return payload.Token, payload.ExpiresAt, nil
}

func copilotTokenExpired(value any) bool {
	switch expiry := value.(type) {
	case float64:
		return expiry > 0 && expiry <= float64(time.Now().Unix())
	case int64:
		return expiry > 0 && expiry <= time.Now().Unix()
	case string:
		if parsed, err := time.Parse(time.RFC3339, expiry); err == nil {
			return !parsed.After(time.Now())
		}
	}
	return true
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := githubStringValue(payload, key); value != "" {
			return value
		}
	}
	return ""
}
