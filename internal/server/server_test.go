package server

import (
	"os"
	"testing"
)

func TestNewUsesConfiguredDatabasePath(t *testing.T) {
	path := t.TempDir() + "/state.db"
	app := New(Options{DatabasePath: path})
	if app.database == nil {
		t.Fatal("database is nil")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database path: %v", err)
	}
	if err := app.database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
}
