package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodexImportTokenPreservesClaims(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	encode := func(value any) string {
		data, _ := json.Marshal(value)
		return base64.RawURLEncoding.EncodeToString(data)
	}
	token := "header." + encode(map[string]any{
		"https://api.openai.com/profile": map[string]any{"email": "user@example.com"},
		"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "workspace-1", "chatgpt_plan_type": "pro"},
	}) + ".signature"
	request := httptest.NewRequest(http.MethodPost, "/api/oauth/codex/import-token", strings.NewReader(`{"accessToken":"`+token+`"}`))
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	provider, ok := app.store.Find("codex")
	if !ok || len(provider.Accounts) != 1 {
		t.Fatalf("provider=%#v", provider)
	}
	account := provider.Accounts[0]
	if account.Email != "user@example.com" || account.Workspace != "workspace-1" || account.Plan != "pro" || account.Name != "user@example.com" {
		t.Fatalf("account=%#v", account)
	}
}
