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
