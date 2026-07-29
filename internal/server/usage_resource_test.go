package server

import (
	"testing"

	"g9router/internal/providers"
)

func TestProviderAccountKeepsSelectedCodexCredentials(t *testing.T) {
	provider := providerAccount(providers.Provider{ID: "codex", APIKey: "parent"}, providers.Account{
		ID: "codex-team", APIKey: "team-token", OAuthID: "team-oauth", Workspace: "team-workspace",
	})
	if provider.APIKey != "team-token" || provider.OAuthID != "team-oauth" || len(provider.Accounts) != 1 {
		t.Fatalf("selected provider = %+v", provider)
	}
	if provider.Accounts[0].Workspace != "team-workspace" {
		t.Fatalf("selected account = %+v", provider.Accounts[0])
	}
}

func TestProviderAccountUsesParentOAuthForLegacyAccount(t *testing.T) {
	provider := providerAccount(providers.Provider{ID: "codex", OAuthID: "current-oauth"}, providers.Account{ID: "codex-KsxIUIcrQNaW"})
	if provider.OAuthID != "current-oauth" {
		t.Fatalf("oauth id = %q", provider.OAuthID)
	}
	legacy := providerAccount(providers.Provider{ID: "codex", OAuthID: "current-oauth"}, providers.Account{ID: "codex-legacy"})
	if legacy.OAuthID != "" {
		t.Fatalf("legacy oauth id = %q", legacy.OAuthID)
	}
}

func TestCodexUsageAccountPrefersSelectedWorkspace(t *testing.T) {
	provider := providerAccount(providers.Provider{
		ID: "codex", ProviderSpecificData: map[string]any{"workspaceId": "parent-workspace"},
	}, providers.Account{ID: "codex-team", Workspace: "team-workspace"})
	if got := codexUsageAccountID(provider, ""); got != "team-workspace" {
		t.Fatalf("workspace = %q", got)
	}
}

func TestCodexAccountIDWinsOverParentProviderID(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "codex", APIKey: "parent", Accounts: []providers.Account{
		{ID: "codex-team", APIKey: "team-token", Workspace: "team"},
		{ID: "codex", APIKey: "primary-token", Workspace: "primary"},
	}}); err != nil {
		t.Fatal(err)
	}
	selected, token, ok := app.codexConnection("codex")
	if !ok || token != "primary-token" || selected.Accounts[0].Workspace != "primary" {
		t.Fatalf("selectedID=%q token=%q ok=%v accounts=%+v", selected.ID, token, ok, selected.Accounts)
	}
}

func TestProviderAccountDoesNotBorrowParentToken(t *testing.T) {
	provider := providerAccount(providers.Provider{ID: "codex", APIKey: "parent-token"}, providers.Account{ID: "codex-team"})
	if provider.APIKey != "" {
		t.Fatalf("token = %q", provider.APIKey)
	}
}
