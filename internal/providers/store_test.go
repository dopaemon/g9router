package providers

import "testing"

func TestResolveRotatesAccountsAndHonorsQuota(t *testing.T) {
	store := New(t.TempDir() + "/providers.json")
	if err := store.Upsert(Provider{ID: "demo", BaseURL: "http://demo", Enabled: true, Accounts: []Account{{ID: "a", APIKey: "key-a", Enabled: true, RequestsLimit: 1}, {ID: "b", APIKey: "key-b", Enabled: true, RequestsLimit: 1}}}); err != nil {
		t.Fatal(err)
	}
	first := store.Resolve("demo/model")
	second := store.Resolve("demo/model")
	third := store.Resolve("demo/model")
	if first[0].APIKey != "key-a" || second[0].APIKey != "key-b" {
		t.Fatalf("rotation = %q, %q", first[0].APIKey, second[0].APIKey)
	}
	if third[0].APIKey != "" {
		t.Fatalf("quota ignored: %q", third[0].APIKey)
	}
}

func TestResolveMatchesProviderAlias(t *testing.T) {
	store := New(t.TempDir() + "/providers.json")
	if err := store.Upsert(Provider{ID: "codex", BaseURL: "http://codex", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	resolved := store.Resolve("cx/gpt-5.5")
	if len(resolved) != 1 || resolved[0].ID != "codex" {
		t.Fatalf("resolved providers = %+v", resolved)
	}
}

func TestListRedactsWithoutMutatingStoredCredentials(t *testing.T) {
	store := New(t.TempDir() + "/providers.json")
	if err := store.Upsert(Provider{ID: "codex", APIKey: "parent", ProviderSpecificData: map[string]any{"region": "ams"}, Accounts: []Account{{ID: "codex", APIKey: "account"}}}); err != nil {
		t.Fatal(err)
	}
	initial, ok := store.Find("codex")
	if !ok || initial.ProviderSpecificData["region"] != "ams" {
		t.Fatalf("initial data = %#v", initial.ProviderSpecificData)
	}
	listed := store.List()
	if listed[0].APIKey != "" || listed[0].Accounts[0].APIKey != "" {
		t.Fatalf("secrets leaked: %+v", listed)
	}
	provider, ok := store.Find("codex")
	if !ok || provider.APIKey != "parent" || provider.Accounts[0].APIKey != "account" || provider.ProviderSpecificData["region"] != "ams" {
		t.Fatalf("stored credentials mutated: %+v", provider)
	}
}

func TestEnabledKeepsCredentialsForInternalRequests(t *testing.T) {
	store := New(t.TempDir() + "/providers.json")
	if err := store.Upsert(Provider{ID: "demo", APIKey: "provider-key", Enabled: true, Accounts: []Account{{ID: "account", APIKey: "account-key"}}}); err != nil {
		t.Fatal(err)
	}
	providers := store.Enabled()
	if len(providers) != 1 || providers[0].APIKey != "provider-key" || providers[0].Accounts[0].APIKey != "account-key" {
		t.Fatalf("enabled credentials = %+v", providers)
	}
}
