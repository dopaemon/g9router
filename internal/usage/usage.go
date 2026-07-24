package usage

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"g9router/internal/db"
)

type Snapshot struct {
	Requests    int64 `json:"requests"`
	Errors      int64 `json:"errors"`
	InputBytes  int64 `json:"inputBytes"`
	OutputBytes int64 `json:"outputBytes"`
}
type Store struct {
	mu       sync.RWMutex
	snapshot Snapshot
	path     string
	database *sql.DB
}

func New(path string) *Store {
	store := &Store{path: path}
	if strings.HasSuffix(path, ".db") {
		if database, err := db.Open(path); err == nil {
			store.database = database
			_ = store.loadDB()
			return store
		}
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &store.snapshot)
	}
	return store
}

func (s *Store) Add(requests, errors, input, output int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Requests += requests
	s.snapshot.Errors += errors
	s.snapshot.InputBytes += input
	s.snapshot.OutputBytes += output
	_ = s.saveLocked()
}
func (s *Store) Snapshot() Snapshot { s.mu.RLock(); defer s.mu.RUnlock(); return s.snapshot }
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = Snapshot{}
	return s.saveLocked()
}
func (s *Store) saveLocked() error {
	if s.database != nil {
		payload, err := json.Marshal(s.snapshot)
		if err != nil {
			return err
		}
		_, err = s.database.Exec(`INSERT INTO usage(id,payload,updated_at) VALUES(1,?,unixepoch()) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, string(payload))
		return err
	}
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s.snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
func (s *Store) loadDB() error {
	var payload string
	err := s.database.QueryRow(`SELECT payload FROM usage WHERE id=1`).Scan(&payload)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(payload), &s.snapshot)
}
