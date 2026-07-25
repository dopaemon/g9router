package server

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexAccountDecodesUnpaddedJWTClaims(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"https://api.openai.com/profile": map[string]any{"email": "user@example.com"},
		"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "workspace", "chatgpt_plan_type": "pro"},
	})
	token := "header." + strings.TrimRight(base64.RawURLEncoding.EncodeToString(payload), "=") + ".signature"
	account := codexAccount(token, "")
	if account.Email != "user@example.com" || account.Workspace != "workspace" || account.Plan != "pro" {
		t.Fatalf("account=%+v", account)
	}
}
