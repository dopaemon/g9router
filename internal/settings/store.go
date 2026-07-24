package settings

import (
	"database/sql"
	"encoding/json"
	"sync"
)

type Store struct {
	mu       sync.RWMutex
	database *sql.DB
	values   map[string]any
}

func New(database *sql.DB) *Store {
	store := &Store{database: database, values: map[string]any{}}
	store.load()
	return store
}
func (s *Store) Get() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[string]any{}
	for key, value := range s.values {
		if key != "password" && key != "apiKey" {
			result[key] = value
		}
	}
	return result
}
func (s *Store) Update(values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range values {
		s.values[key] = value
	}
	payload, err := json.Marshal(s.values)
	if err != nil {
		return err
	}
	_, err = s.database.Exec(`INSERT INTO settings(id,payload,updated_at) VALUES(1,?,unixepoch()) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, string(payload))
	return err
}
func (s *Store) load() {
	var payload string
	if s.database.QueryRow(`SELECT payload FROM settings WHERE id=1`).Scan(&payload) == nil {
		_ = json.Unmarshal([]byte(payload), &s.values)
	}
}
