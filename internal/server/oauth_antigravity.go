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
	antigravityAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	antigravityTokenURL     = "https://oauth2.googleapis.com/token"
	antigravityUserInfoURL  = "https://www.googleapis.com/oauth2/v1/userinfo"
	antigravityLoadURL      = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	antigravityOnboardURL   = "https://cloudcode-pa.googleapis.com/v1internal:onboardUser"
)

func (s *Server) antigravityAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if os.Getenv("G9ROUTER_ANTIGRAVITY_CLIENT_ID") == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "G9ROUTER_ANTIGRAVITY_CLIENT_ID is not configured"})
		return
	}
	redirect := r.URL.Query().Get("redirect_uri")
	if redirect == "" {
		redirect = "http://localhost:8080/callback"
	}
	state, err := randomURLToken(24)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	scope := "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile https://www.googleapis.com/auth/cclog https://www.googleapis.com/auth/experimentsandconfigs"
	query := url.Values{"client_id": {os.Getenv("G9ROUTER_ANTIGRAVITY_CLIENT_ID")}, "response_type": {"code"}, "redirect_uri": {redirect}, "scope": {scope}, "state": {state}, "access_type": {"offline"}, "prompt": {"consent"}}
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": antigravityAuthorizeURL + "?" + query.Encode(), "state": state, "redirectUri": redirect, "provider": "antigravity"})
}

func (s *Server) antigravityExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	clientID, clientSecret := os.Getenv("G9ROUTER_ANTIGRAVITY_CLIENT_ID"), os.Getenv("G9ROUTER_ANTIGRAVITY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Antigravity OAuth client credentials are not configured"})
		return
	}
	var input struct {
		Code        string `json:"code"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || input.Code == "" || input.RedirectURI == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "client_secret": {clientSecret}, "code": {input.Code}, "redirect_uri": {input.RedirectURI}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, antigravityTokenURL, strings.NewReader(form.Encode()))
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
	var token map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Token exchange failed"})
		return
	}
	accessToken := antigravityString(token, "access_token")
	if accessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid token response"})
		return
	}
	profile, projectID, tierID := s.antigravityMetadata(r, accessToken)
	credentialID := "antigravity-oauth"
	expiresIn := antigravityInt64(token, "expires_in")
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "antigravity", AccessToken: accessToken, RefreshToken: antigravityString(token, "refresh_token"), TokenURL: antigravityTokenURL, ClientID: clientID, ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli(), Scope: antigravityString(token, "scope")}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	providerData := map[string]any{"email": profile["email"], "projectId": projectID, "tierId": tierID}
	if err := s.store.Upsert(providers.Provider{ID: "antigravity", Name: "Antigravity", BaseURL: "https://cloudcode-pa.googleapis.com", APIKey: accessToken, APIType: "antigravity", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tokens": map[string]any{"accessToken": accessToken, "refreshToken": antigravityString(token, "refresh_token"), "expiresIn": expiresIn, "email": profile["email"], "projectId": projectID, "tierId": tierID}, "connection": map[string]string{"provider": "antigravity", "id": credentialID}})
}

func (s *Server) antigravityMetadata(r *http.Request, accessToken string) (map[string]any, string, string) {
	profile := map[string]any{}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, antigravityUserInfoURL+"?alt=json", nil)
	if err == nil {
		request.Header.Set("Authorization", "Bearer "+accessToken)
		request.Header.Set("x-request-source", "local")
		if response, requestErr := s.client.Do(request); requestErr == nil {
			_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile)
			response.Body.Close()
		}
	}
	metadata := `{"ideType":9,"platform":1,"pluginType":2}`
	request, err = http.NewRequestWithContext(r.Context(), http.MethodPost, antigravityLoadURL, strings.NewReader(`{"metadata":{"ideType":9,"platform":1,"pluginType":2}}`))
	if err != nil {
		return profile, "", "legacy-tier"
	}
	for key, value := range map[string]string{"Authorization": "Bearer " + accessToken, "Content-Type": "application/json", "User-Agent": "google-api-nodejs-client/9.15.1", "X-Goog-Api-Client": "google-cloud-sdk vscode_cloudshelleditor/0.1", "Client-Metadata": metadata, "x-request-source": "local"} {
		request.Header.Set(key, value)
	}
	projectID, tierID := "", "legacy-tier"
	if response, requestErr := s.client.Do(request); requestErr == nil {
		var payload map[string]any
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload)
		response.Body.Close()
		if project, ok := payload["cloudaicompanionProject"].(map[string]any); ok {
			projectID, _ = project["id"].(string)
		} else if project, ok := payload["cloudaicompanionProject"].(string); ok {
			projectID = project
		}
		if tiers, ok := payload["allowedTiers"].([]any); ok {
			for _, item := range tiers {
				if tier, ok := item.(map[string]any); ok && tier["isDefault"] == true {
					if value, ok := tier["id"].(string); ok && strings.TrimSpace(value) != "" {
						tierID = strings.TrimSpace(value)
					}
				}
			}
		}
	}
	if projectID != "" {
		go s.antigravityOnboard(projectID, tierID, accessToken)
	}
	return profile, projectID, tierID
}

func (s *Server) antigravityOnboard(projectID, tierID, accessToken string) {
	body := `{"tierId":"` + tierID + `","metadata":{"ideType":9,"platform":1,"pluginType":2}}`
	request, err := http.NewRequest(http.MethodPost, antigravityOnboardURL, strings.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "google-api-nodejs-client/9.15.1")
	for attempt := 0; attempt < 10; attempt++ {
		response, requestErr := s.client.Do(request)
		if requestErr != nil {
			return
		}
		var result map[string]any
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
		response.Body.Close()
		if result["done"] == true {
			return
		}
		time.Sleep(5 * time.Second)
	}
}

func antigravityString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func antigravityInt64(payload map[string]any, key string) int64 {
	value, _ := payload[key].(float64)
	return int64(value)
}
