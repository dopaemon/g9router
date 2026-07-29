package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"g9router/internal/providers"
)

func TestCodexAccountDecodesUnpaddedJWTClaims(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"https://api.openai.com/profile": map[string]any{"email": "user@example.com"},
		"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "workspace", "chatgpt_plan_type": "pro"},
	})
	token := "header." + strings.TrimRight(base64.RawURLEncoding.EncodeToString(payload), "=") + ".signature"
	account := codexAccount(token, "")
	if account.Email != "user@example.com" || account.Name != "Codex user@example.com" || account.Workspace != "workspace" || account.Plan != "pro" {
		t.Fatalf("account=%+v", account)
	}
}

func TestCodexAccountsKeepSameEmailAcrossPlans(t *testing.T) {
	email := "user@example.com"
	teamPayload, _ := json.Marshal(map[string]any{
		"https://api.openai.com/profile": map[string]any{"email": email},
		"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "team", "chatgpt_plan_type": "team"},
	})
	personalPayload, _ := json.Marshal(map[string]any{
		"https://api.openai.com/profile": map[string]any{"email": email},
		"https://api.openai.com/auth":    map[string]any{"chatgpt_account_id": "personal", "chatgpt_plan_type": "pro"},
	})
	team := codexAccountFromTokens("team-access", jwtPayload(teamPayload), "")
	personal := codexAccountFromTokens("personal-access", jwtPayload(personalPayload), "")
	if team.Email != personal.Email || team.ID != "codex-team" || personal.ID != "codex" || team.Plan == personal.Plan {
		t.Fatalf("accounts=%+v %+v", team, personal)
	}
}

func jwtPayload(payload []byte) string {
	return "header." + strings.TrimRight(base64.RawURLEncoding.EncodeToString(payload), "=") + ".signature"
}

func TestCodexAccountResourceDeleteKeepsSiblingPlan(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "codex", Accounts: []providers.Account{
		{ID: "codex-team", Name: "Team", Enabled: true},
		{ID: "codex-pro", Name: "Personal", Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/providers/codex-team", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	provider, _ := app.store.Find("codex")
	if len(provider.Accounts) != 1 || provider.Accounts[0].ID != "codex-pro" {
		t.Fatalf("accounts=%+v", provider.Accounts)
	}
}

func TestProviderQueryDeleteRemovesCodexAccount(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "codex", Accounts: []providers.Account{
		{ID: "codex-team", Enabled: true},
		{ID: "codex", Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/providers?id=codex-team", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	provider, _ := app.store.Find("codex")
	if len(provider.Accounts) != 1 || provider.Accounts[0].ID != "codex" {
		t.Fatalf("accounts=%+v", provider.Accounts)
	}
}

func TestProviderQueryDeleteRemovesCodexPrimaryAccount(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "codex", Accounts: []providers.Account{
		{ID: "codex-team", Enabled: true},
		{ID: "codex", Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/providers?id=codex", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	provider, _ := app.store.Find("codex")
	if len(provider.Accounts) != 1 || provider.Accounts[0].ID != "codex-team" {
		t.Fatalf("accounts=%+v", provider.Accounts)
	}
}
