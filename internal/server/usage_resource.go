package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/providers"
)

func (s *Server) usageResourceAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/usage/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "codex-reset-credits" {
		provider, token, ok := s.codexConnection(parts[0])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Codex connection not found"})
			return
		}
		if provider.ID != "codex" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Codex reset credits are only available for Codex connections."})
			return
		}
		if token == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Codex reset credits require an OAuth or access-token connection."})
			return
		}
		if r.Method == http.MethodGet {
			s.codexCreditsAPI(w, r, token)
			return
		}
		if r.Method == http.MethodPost {
			s.codexConsumeCreditAPI(w, r, token)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		return
	}
	if len(parts) != 1 || parts[0] == "" || r.Method != http.MethodGet {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "usage connection not found"})
		return
	}
	connectionID := parts[0]
	providerName := ""
	var selected providers.Provider
	for _, provider := range s.store.List() {
		if provider.ID == connectionID {
			providerName = provider.ID
			selected = provider
			break
		}
		for _, account := range provider.Accounts {
			if account.ID == connectionID {
				providerName = provider.ID
				selected = provider
				break
			}
		}
		if providerName != "" {
			break
		}
	}
	if providerName == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
		return
	}
	if selected.OAuthID != "" {
		if credential, ok := s.oauth.Get(selected.OAuthID); ok {
			if credential.ExpiringSoon(time.Now()) && credential.RefreshToken != "" {
				if refreshed, err := s.oauth.Refresh(r.Context(), selected.OAuthID); err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Credential refresh failed: " + err.Error()})
					return
				} else {
					selected.APIKey = refreshed.AccessToken
				}
			}
		}
	}
	logs := s.usage.Recent(1000)
	var requests, input, output, errors int64
	for _, entry := range logs {
		if entry.Provider != providerName {
			continue
		}
		requests++
		input += entry.Input
		output += entry.Output
		if entry.Status != "" && entry.Status != "ok" {
			errors++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectionId": connectionID, "provider": providerName, "requests": requests, "errors": errors, "inputTokens": input, "outputTokens": output, "source": "g9router usage log"})
}

func (s *Server) codexConnection(id string) (providers.Provider, string, bool) {
	for _, provider := range s.store.List() {
		if provider.ID == id {
			if provider.APIKey != "" {
				return provider, provider.APIKey, true
			}
			for _, account := range provider.Accounts {
				if account.APIKey != "" {
					return provider, account.APIKey, true
				}
			}
			return provider, "", true
		}
		for _, account := range provider.Accounts {
			if account.ID == id {
				return provider, account.APIKey, true
			}
		}
	}
	return providers.Provider{}, "", false
}

func (s *Server) codexCreditsAPI(w http.ResponseWriter, r *http.Request, token string) {
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("OpenAI-Beta", "codex-1")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, response.StatusCode, map[string]string{"error": strings.TrimSpace(string(data))})
		return
	}
	var payload struct {
		AvailableCount int              `json:"available_count"`
		Credits        []map[string]any `json:"credits"`
	}
	if json.Unmarshal(data, &payload) != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid Codex credits response"})
		return
	}
	writeJSON(w, 200, map[string]any{"availableCount": payload.AvailableCount, "credits": payload.Credits})
}

func (s *Server) codexConsumeCreditAPI(w http.ResponseWriter, r *http.Request, token string) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	redeemID := hex.EncodeToString(idBytes)
	body, _ := json.Marshal(map[string]string{"redeem_request_id": redeemID})
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits/consume", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	var payload map[string]any
	_ = json.Unmarshal(data, &payload)
	code, _ := payload["code"].(string)
	windows, _ := payload["windows_reset"].(float64)
	if response.StatusCode >= 200 && response.StatusCode < 300 && (code == "reset" || windows > 0) {
		writeJSON(w, 200, map[string]any{"code": code, "reset": true, "windows_reset": windows, "redeemRequestId": redeemID, "credit": payload["credit"]})
		return
	}
	if code == "no_credit" {
		writeJSON(w, 409, map[string]any{"code": code, "reset": false, "windows_reset": windows, "message": "No Codex reset credits available."})
		return
	}
	writeJSON(w, 502, map[string]any{"code": code, "reset": false, "windows_reset": windows, "message": payload["message"]})
}
