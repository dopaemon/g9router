package settings

import (
	"testing"

	"g9router/internal/db"
)

func BenchmarkUpdate(b *testing.B) {
	database, err := db.Open(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer database.Close()

	store := New(database)
	values := map[string]any{"locale": "vi", "theme": "dark", "refresh": 2}
	b.ReportAllocs()
	for b.Loop() {
		if err := store.Update(values); err != nil {
			b.Fatal(err)
		}
	}
}
