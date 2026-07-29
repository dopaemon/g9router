package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) codexImportTokenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		AccessToken string `json:"accessToken"`
		Name        string `json:"name"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&input) != nil || strings.TrimSpace(input.AccessToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Access token is required"})
		return
	}
	account := codexAccount(strings.TrimSpace(input.AccessToken), input.Name)
	provider, _ := s.store.Find("codex")
	provider.ID, provider.Name = "codex", "Codex"
	if provider.BaseURL == "" {
		if descriptor, ok := providers.Lookup("codex"); ok {
			provider.BaseURL, provider.APIType = descriptor.BaseURL, "codex"
		}
	}
	provider.Enabled = true
	provider.Accounts = upsertCodexAccount(provider.Accounts, account)
	if err := s.store.Upsert(provider); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "connection": map[string]any{"id": account.ID, "provider": "codex", "email": account.Email, "name": account.Name, "workspace": account.Workspace, "plan": account.Plan}})
}

func (s *Server) codexBulkImportAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var raw any
	if json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&raw) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}
	items := []any{}
	switch value := raw.(type) {
	case []any:
		items = value
	case map[string]any:
		if accounts, ok := value["accounts"].([]any); ok {
			items = accounts
		} else {
			items = []any{value}
		}
	}
	if len(items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No accounts provided"})
		return
	}
	provider, _ := s.store.Find("codex")
	provider.ID, provider.Name, provider.Enabled = "codex", "Codex", true
	if provider.BaseURL == "" {
		if descriptor, ok := providers.Lookup("codex"); ok {
			provider.BaseURL, provider.APIType = descriptor.BaseURL, "codex"
		}
	}
	results := make([]map[string]any, 0, len(items))
	success := 0
	for index, value := range items {
		item, ok := value.(map[string]any)
		token, _ := item["accessToken"].(string)
		if !ok || strings.TrimSpace(token) == "" {
			results = append(results, map[string]any{"index": index, "ok": false, "error": "Missing accessToken"})
			continue
		}
		account := codexAccount(strings.TrimSpace(token), stringValue(item["name"]))
		provider.Accounts = upsertCodexAccount(provider.Accounts, account)
		results = append(results, map[string]any{"index": index, "ok": true, "id": account.ID})
		success++
	}
	if err := s.store.Upsert(provider); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": success, "failed": len(items) - success, "results": results})
}

func upsertCodexAccount(accounts []providers.Account, account providers.Account) []providers.Account {
	updated := false
	filtered := accounts[:0]
	for index := range accounts {
		if accounts[index].ID == account.ID {
			if !updated {
				filtered = append(filtered, account)
				updated = true
			}
			continue
		}
		filtered = append(filtered, accounts[index])
	}
	if !updated {
		filtered = append(filtered, account)
	}
	return filtered
}

func codexAccount(token, name string) providers.Account {
	return codexAccountFromTokens(token, token, name)
}

func codexAccountFromTokens(accessToken, claimsToken, name string) providers.Account {
	email, workspace, plan := codexClaims(claimsToken)
	account := providers.Account{ID: codexAccountID(plan), APIKey: accessToken, Name: strings.TrimSpace(name), Email: email, Workspace: workspace, Plan: plan, Enabled: true}
	if len(account.Name) == 0 {
		account.Name = "ChatGPT Access Token"
	}
	if account.Email != "" {
		account.Name = "Codex " + account.Email
	}
	return account
}

func codexAccountID(plan string) string {
	plan = strings.ToLower(strings.TrimSpace(plan))
	if strings.Contains(plan, "team") || strings.Contains(plan, "business") || strings.Contains(plan, "enterprise") {
		return "codex-team"
	}
	return "codex"
}

func codexClaims(token string) (email, workspace, plan string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", ""
	}
	encoded := strings.ReplaceAll(strings.ReplaceAll(parts[1], "-", "+"), "_", "/")
	encoded += strings.Repeat("=", (4-len(encoded)%4)%4)
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", ""
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return "", "", ""
	}
	profile, _ := claims["https://api.openai.com/profile"].(map[string]any)
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	return codexString(profile["email"], claims["email"], claims["preferred_username"]), codexString(auth["chatgpt_account_id"]), codexString(auth["chatgpt_plan_type"])
}

func codexString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func stringValue(value any) string { result, _ := value.(string); return result }
