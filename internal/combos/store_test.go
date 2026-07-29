package combos

import "testing"

func TestCreateKeepsComboIDsUniqueAfterDelete(t *testing.T) {
	store := New(t.TempDir() + "/combos.json")
	first, err := store.Create("first", []any{"model-a"}, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("second", []any{"model-b"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Delete(first.ID); err != nil || !removed {
		t.Fatalf("delete = %v, %v", removed, err)
	}
	third, err := store.Create("third", []any{"model-c"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == first.ID || third.ID == second.ID {
		t.Fatalf("reused combo ID: first=%q second=%q third=%q", first.ID, second.ID, third.ID)
	}
}
