package db

import "testing"

func TestBackupRoundTrip(t *testing.T) {
	database, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`INSERT INTO settings(id,payload,updated_at) VALUES(1,'{"requireLogin":true}',unixepoch())`); err != nil {
		t.Fatal(err)
	}
	backup, err := Export(database)
	if err != nil || backup.Settings["1"] == "" {
		t.Fatal(backup, err)
	}
	if _, err := database.Exec(`DELETE FROM settings`); err != nil {
		t.Fatal(err)
	}
	if err := Import(database, backup); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := database.QueryRow(`SELECT payload FROM settings WHERE id=1`).Scan(&payload); err != nil || payload != `{"requireLogin":true}` {
		t.Fatal(payload, err)
	}
}
