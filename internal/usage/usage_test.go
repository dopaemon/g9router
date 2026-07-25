package usage

import "testing"

func TestStorePersistsAndResets(t *testing.T) {
	path := t.TempDir() + "/usage.json"
	store := New(path)
	store.Add(1, 0, 10, 20)
	restored := New(path)
	if restored.Snapshot().Requests != 1 || restored.Snapshot().OutputBytes != 20 {
		t.Fatal(restored.Snapshot())
	}
	if err := restored.Reset(); err != nil {
		t.Fatal(err)
	}
	if restored.Snapshot().Requests != 0 {
		t.Fatal(restored.Snapshot())
	}
}

func TestProvidersAreSorted(t *testing.T) {
	store := New(t.TempDir() + "/usage.json")
	store.AddLog(1, 0, 0, 0, Log{Provider: "z"})
	store.AddLog(1, 0, 0, 0, Log{Provider: "a"})
	providers := store.Providers()
	if providers[0]["id"] != "a" || providers[1]["id"] != "z" {
		t.Fatal(providers)
	}
}
