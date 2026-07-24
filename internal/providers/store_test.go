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
