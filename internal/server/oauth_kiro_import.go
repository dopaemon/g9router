package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/providers"
)

func (s *Server) kiroImportAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		RefreshToken string `json:"refreshToken"`
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
		Region       string `json:"region"`
		AuthMethod   string `json:"authMethod"`
		ProfileARN   string `json:"profileArn"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.RefreshToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Refresh token is required"})
		return
	}
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if !strings.HasPrefix(refreshToken, "aorAAAAAG") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid token format. Token should start with aorAAAAAG..."})
		return
	}
	region := input.Region
	if region == "" {
		region = "us-east-1"
	}
	endpoint := "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"
	payload := map[string]string{"refreshToken": refreshToken}
	if input.ClientID != "" && input.ClientSecret != "" {
		endpoint = "https://oidc." + region + ".amazonaws.com/token"
		payload = map[string]string{"clientId": input.ClientID, "clientSecret": input.ClientSecret, "refreshToken": refreshToken, "grantType": "refresh_token"}
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Token validation failed"})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Token validation failed"})
		return
	}
	var token struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ProfileARN   string `json:"profileArn"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if json.Unmarshal(data, &token) != nil || token.AccessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid token response"})
		return
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	profileARN := input.ProfileARN
	if profileARN == "" {
		profileARN = token.ProfileARN
	}
	authMethod := input.AuthMethod
	if input.ClientID != "" && input.ClientSecret != "" {
		authMethod = "idc"
	}
	if authMethod == "" {
		authMethod = "imported"
	}
	dataMap := map[string]any{"profileArn": profileARN, "authMethod": authMethod, "provider": "Imported"}
	if authMethod == "idc" {
		dataMap["clientId"], dataMap["clientSecret"], dataMap["region"] = input.ClientID, input.ClientSecret, region
	}
	if err := s.store.Upsert(providers.Provider{ID: "kiro", Name: "Kiro", BaseURL: "https://runtime."+region+".kiro.dev", APIKey: token.AccessToken, APIType: "kiro", Enabled: true, TestStatus: "active", ProviderSpecificData: dataMap, Accounts: []providers.Account{{ID: "kiro-imported", APIKey: token.AccessToken, Enabled: true}}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "kiro", "expiresAt": time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC().Format(time.RFC3339), "profileArn": profileARN})
}
