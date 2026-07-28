package settings

import "testing"

func TestStoreWithoutDatabaseDoesNotPanic(t *testing.T) {
	store := New(nil)
	store.Reload()
	if err := store.Update(map[string]any{"locale": "vi"}); err == nil {
		t.Fatal("Update should report unavailable database")
	}
}
