package server

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"g9router/internal/providers"
)

var awsRegionPattern = regexp.MustCompile(`^[a-z]{2}(-gov)?-[a-z]+-\d$`)

func (s *Server) kiroAPIKeyAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		APIKey string `json:"apiKey"`
		Region string `json:"region"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.APIKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "API key is required"})
		return
	}
	region := strings.TrimSpace(input.Region)
	if region == "" {
		region = "us-east-1"
	}
	if !awsRegionPattern.MatchString(region) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid AWS region"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://codewhisperer."+region+".amazonaws.com", strings.NewReader(`{"maxResults":10}`))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Content-Type", "application/x-amz-json-1.0")
	request.Header.Set("x-amz-target", "AmazonCodeWhispererService.ListAvailableProfiles")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(input.APIKey))
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "API key validation failed"})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "API key validation failed"})
		return
	}
	var payload struct {
		Profiles []struct {
			ARN        string `json:"arn"`
			ProfileARN string `json:"profileArn"`
		} `json:"profiles"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Profiles) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "API key validation failed"})
		return
	}
	profileARN := payload.Profiles[0].ARN
	for _, profile := range payload.Profiles {
		candidate := profile.ARN
		if candidate == "" {
			candidate = profile.ProfileARN
		}
		if strings.Contains(candidate, ":"+region+":") {
			profileARN = candidate
			break
		}
	}
	if profileARN == "" {
		profileARN = payload.Profiles[0].ProfileARN
	}
	if err := s.store.Upsert(providers.Provider{ID: "kiro", Name: "Kiro", BaseURL: "https://runtime."+region+".kiro.dev", APIKey: strings.TrimSpace(input.APIKey), APIType: "kiro", Enabled: true, TestStatus: "active", ProviderSpecificData: map[string]any{"profileArn": profileARN, "region": region, "authMethod": "api_key", "provider": "API Key"}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "kiro", "profileArn": profileARN})
}
