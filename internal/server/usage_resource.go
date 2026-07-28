package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/providers"
)

var usageAPIKeyProviders = map[string]bool{
	"vercel-ai-gateway": true,
	"codebuddy-cn":      true,
	"minimax":           true,
	"glm":               true,
	"minimax-cn":        true,
	"glm-cn":            true,
	"kiro":              true,
}

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
			s.codexCreditsAPI(w, r, token, codexUsageAccountID(provider, token))
			return
		}
		if r.Method == http.MethodPost {
			available, err := s.codexAvailableResetCredits(r.Context(), token, codexUsageAccountID(provider, token))
			if err != nil {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			if available < 1 {
				writeJSON(w, http.StatusConflict, map[string]any{"code": "no_credit", "reset": false, "availableCount": available, "message": "No Codex reset credits available."})
				return
			}
			s.codexConsumeCreditAPI(w, r, token, codexUsageAccountID(provider, token))
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
	for _, visible := range s.store.List() {
		provider, found := s.store.Find(visible.ID)
		if !found {
			continue
		}
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
	if selected.OAuthID == "" && !usageAPIKeyProviders[providerName] && providerName != "codex" {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Usage not available for this connection"})
		return
	}
	var err error
	selected, err = s.credentialProvider(r.Context(), selected, true)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Credential refresh failed: " + err.Error()})
		return
	}
	if selected.ID == "codex" {
		s.codexUsageAPI(w, r, selected)
		return
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

func (s *Server) codexUsageAPI(w http.ResponseWriter, r *http.Request, provider providers.Provider) {
	token := provider.APIKey
	if token == "" {
		for _, account := range provider.Accounts {
			if account.APIKey != "" {
				token = account.APIKey
				break
			}
		}
	}
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Codex token unavailable"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
	request.Header.Set("OpenAI-Beta", "codex-1")
	if accountID := codexUsageAccountID(provider, token); accountID != "" {
		request.Header.Set("ChatGPT-Account-ID", accountID)
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, response.StatusCode, map[string]string{"error": "Codex quota API returned " + response.Status})
		return
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid Codex quota response"})
		return
	}
	rateLimit := mapValue(payload, "rate_limit")
	if len(rateLimit) == 0 {
		rateLimit = mapValue(payload, "rate_limits")
	}
	if len(rateLimit) == 0 {
		rateLimit = mapValue(mapValue(payload, "rate_limits_by_limit_id"), "codex")
	}
	quotas := map[string]any{}
	for _, window := range []struct {
		name string
		key  string
	}{{"session", "primary_window"}, {"weekly", "secondary_window"}} {
		value := mapValue(rateLimit, window.key)
		if len(value) == 0 {
			continue
		}
		used := numberValue(value["used_percent"])
		if used == 0 {
			used = numberValue(value["percent_used"])
		}
		resetAt := codexResetTime(value["reset_at"])
		if resetAt == "" {
			resetAt = codexResetTime(value["resets_at"])
		}
		quotas[window.name] = map[string]any{"used": used, "total": 100, "remaining": 100 - used, "resetAt": resetAt}
	}
	plan := payload["plan_type"]
	if plan == nil {
		if summary := mapValue(payload, "summary"); summary != nil {
			plan = summary["plan"]
		}
	}
	result := map[string]any{"provider": "codex", "plan": plan, "quotas": quotas}
	if available, err := s.codexAvailableResetCredits(r.Context(), token, codexUsageAccountID(provider, token)); err == nil {
		result["resetCredits"] = map[string]any{"availableCount": available}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) codexAvailableResetCredits(ctx context.Context, token, accountID string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits", nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("OpenAI-Beta", "codex-1")
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
	if accountID != "" {
		request.Header.Set("ChatGPT-Account-ID", accountID)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("Codex reset credits API returned %s", response.Status)
	}
	var payload struct {
		AvailableCount int `json:"available_count"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return 0, err
	}
	return max(0, payload.AvailableCount), nil
}

func codexUsageAccountID(provider providers.Provider, token string) string {
	for _, key := range []string{"workspaceId", "accountId", "chatgptAccountId"} {
		if value, ok := provider.ProviderSpecificData[key].(string); ok && value != "" {
			return value
		}
	}
	for _, account := range provider.Accounts {
		if account.Workspace != "" {
			return account.Workspace
		}
	}
	return codexAccount(token, "").Workspace
}

func mapValue(value map[string]any, key string) map[string]any {
	result, _ := value[key].(map[string]any)
	return result
}

func numberValue(value any) float64 {
	switch value := value.(type) {
	case float64:
		return max(0, min(100, value))
	case int:
		return max(0, min(100, float64(value)))
	case json.Number:
		parsed, _ := value.Float64()
		return max(0, min(100, parsed))
	default:
		return 0
	}
}

func codexResetTime(value any) string {
	seconds := 0.0
	switch value := value.(type) {
	case float64:
		seconds = value
	case int:
		seconds = float64(value)
	case json.Number:
		seconds, _ = value.Float64()
	}
	if seconds == 0 {
		return ""
	}
	return time.Unix(int64(seconds), 0).UTC().Format(time.RFC3339)
}

func (s *Server) codexConnection(id string) (providers.Provider, string, bool) {
	for _, visible := range s.store.List() {
		provider, found := s.store.Find(visible.ID)
		if !found {
			continue
		}
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

func (s *Server) codexCreditsAPI(w http.ResponseWriter, r *http.Request, token, accountID string) {
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("OpenAI-Beta", "codex-1")
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
	if accountID != "" {
		request.Header.Set("ChatGPT-Account-ID", accountID)
	}
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

func (s *Server) codexConsumeCreditAPI(w http.ResponseWriter, r *http.Request, token, accountID string) {
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
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
	if accountID != "" {
		request.Header.Set("ChatGPT-Account-ID", accountID)
	}
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
