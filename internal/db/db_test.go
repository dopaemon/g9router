package db

import "testing"

func TestOpenCreatesSchema(t *testing.T) {
	database, err := Open(t.TempDir() + "/g9router.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='providers'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal(count)
	}
}
