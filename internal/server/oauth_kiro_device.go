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

func (s *Server) kiroDeviceCodeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Region     string `json:"region"`
		StartURL   string `json:"startUrl"`
		AuthMethod string `json:"authMethod"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input)
	region := strings.TrimSpace(input.Region)
	if region == "" {
		region = "us-east-1"
	}
	if !awsRegionPattern.MatchString(region) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid region"})
		return
	}
	startURL := strings.TrimSpace(input.StartURL)
	if startURL == "" {
		startURL = "https://view.awsapps.com/start"
	}
	registerURL := "https://oidc." + region + ".amazonaws.com/client/register"
	deviceURL := "https://oidc." + region + ".amazonaws.com/device_authorization"
	registerPayload := map[string]any{"clientName": "kiro-oauth-client", "clientType": "public", "scopes": []string{"codewhisperer:completions", "codewhisperer:analysis", "codewhisperer:conversations"}, "grantTypes": []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"}, "issuerUrl": "https://identitycenter.amazonaws.com/ssoins-722374e8c3c8e6c6"}
	clientInfo, status, err := s.kiroJSONRequest(r, http.MethodPost, registerURL, registerPayload)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": "Client registration failed: " + err.Error()})
		return
	}
	devicePayload := map[string]any{"clientId": clientInfo["clientId"], "clientSecret": clientInfo["clientSecret"], "startUrl": startURL}
	deviceInfo, status, err := s.kiroJSONRequest(r, http.MethodPost, deviceURL, devicePayload)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": "Device authorization failed: " + err.Error()})
		return
	}
	method := input.AuthMethod
	if method != "idc" {
		method = "builder-id"
	}
	writeJSON(w, http.StatusOK, map[string]any{"device_code": deviceInfo["deviceCode"], "user_code": deviceInfo["userCode"], "verification_uri": deviceInfo["verificationUri"], "verification_uri_complete": deviceInfo["verificationUriComplete"], "expires_in": deviceInfo["expiresIn"], "interval": deviceInfo["interval"], "_clientId": clientInfo["clientId"], "_clientSecret": clientInfo["clientSecret"], "_region": region, "_authMethod": method, "_startUrl": startURL})
}

func (s *Server) kiroPollAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		DeviceCode   string `json:"device_code"`
		ClientID     string `json:"_clientId"`
		ClientSecret string `json:"_clientSecret"`
		Region       string `json:"_region"`
		AuthMethod   string `json:"_authMethod"`
		StartURL     string `json:"_startUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.DeviceCode == "" || input.ClientID == "" || input.ClientSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing Kiro device authorization data"})
		return
	}
	region := input.Region
	if region == "" {
		region = "us-east-1"
	}
	if !awsRegionPattern.MatchString(region) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid region"})
		return
	}
	payload, status, err := s.kiroJSONRequest(r, http.MethodPost, "https://oidc."+region+".amazonaws.com/token", map[string]any{"clientId": input.ClientID, "clientSecret": input.ClientSecret, "deviceCode": input.DeviceCode, "grantType": "urn:ietf:params:oauth:grant-type:device_code"})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": payload["error"], "errorDescription": payload["error_description"]})
		return
	}
	accessToken := kiroDeviceString(payload, "accessToken")
	if accessToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "error": kiroDeviceString(payload, "error"), "errorDescription": kiroDeviceString(payload, "error_description"), "pending": true})
		return
	}
	expiresIn := kiroDeviceInt64(payload, "expiresIn", 3600)
	credentialID := "kiro-oauth"
	providerData := map[string]any{"profileArn": payload["profileArn"], "clientId": input.ClientID, "clientSecret": input.ClientSecret, "region": region, "authMethod": input.AuthMethod, "startUrl": input.StartURL}
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "kiro", AccessToken: accessToken, RefreshToken: kiroDeviceString(payload, "refreshToken"), TokenURL: "https://oidc." + region + ".amazonaws.com/token", ClientID: input.ClientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "kiro", Name: "Kiro", BaseURL: "https://runtime." + region + ".kiro.dev", APIKey: accessToken, APIType: "kiro", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": kiroDeviceString(payload, "refreshToken"), "expiresIn": expiresIn, "providerSpecificData": providerData}, "connection": map[string]string{"provider": "kiro", "id": credentialID}})
	_ = status
}

func (s *Server) kiroJSONRequest(r *http.Request, method, endpoint string, body any) (map[string]any, int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	request, err := http.NewRequestWithContext(r.Context(), method, endpoint, strings.NewReader(string(data)))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer response.Body.Close()
	var payload map[string]any
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload) != nil {
		return nil, http.StatusBadGateway, io.ErrUnexpectedEOF
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return payload, response.StatusCode, io.ErrUnexpectedEOF
	}
	return payload, http.StatusOK, nil
}

func kiroDeviceString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func kiroDeviceInt64(payload map[string]any, key string, fallback int64) int64 {
	value, ok := payload[key].(float64)
	if !ok || value <= 0 {
		return fallback
	}
	return int64(value)
}
