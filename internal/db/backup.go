package db

import (
	"database/sql"
	"encoding/json"
)

type Backup struct {
	Providers map[string]string `json:"providers"`
	OAuth     map[string]string `json:"oauth"`
	Settings  map[string]string `json:"settings"`
	Usage     map[string]string `json:"usage"`
}

func Export(database *sql.DB) (Backup, error) {
	backup := Backup{Providers: map[string]string{}, OAuth: map[string]string{}, Settings: map[string]string{}, Usage: map[string]string{}}
	for table, target := range map[string]map[string]string{"providers": backup.Providers, "oauth_credentials": backup.OAuth, "settings": backup.Settings, "usage": backup.Usage} {
		rows, err := database.Query("SELECT id, payload FROM " + table)
		if err != nil {
			return Backup{}, err
		}
		for rows.Next() {
			var id, payload string
			if err := rows.Scan(&id, &payload); err != nil {
				rows.Close()
				return Backup{}, err
			}
			target[id] = payload
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return Backup{}, err
		}
		rows.Close()
	}
	return backup, nil
}

func Import(database *sql.DB, backup Backup) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for table, values := range map[string]map[string]string{"providers": backup.Providers, "oauth_credentials": backup.OAuth, "settings": backup.Settings, "usage": backup.Usage} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
		for id, payload := range values {
			if _, err := tx.Exec("INSERT INTO "+table+"(id,payload,updated_at) VALUES(?,?,unixepoch())", id, payload); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (b Backup) Valid() bool {
	_, err := json.Marshal(b)
	return err == nil
}
