package usage

import (
	"database/sql"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"g9router/internal/db"
)

type Snapshot struct {
	Requests    int64 `json:"requests"`
	Errors      int64 `json:"errors"`
	InputBytes  int64 `json:"inputBytes"`
	OutputBytes int64 `json:"outputBytes"`
	Logs        []Log `json:"logs,omitempty"`
}
type Log struct {
	Timestamp string `json:"timestamp"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status,omitempty"`
	Input     int64  `json:"inputTokens,omitempty"`
	Output    int64  `json:"outputTokens,omitempty"`
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
	s.AddLog(requests, errors, input, output, Log{Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
}
func (s *Store) AddLog(requests, errors, input, output int64, log Log) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Requests += requests
	s.snapshot.Errors += errors
	s.snapshot.InputBytes += input
	s.snapshot.OutputBytes += output
	if log.Timestamp == "" {
		log.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	s.snapshot.Logs = append(s.snapshot.Logs, log)
	if len(s.snapshot.Logs) > 1000 {
		s.snapshot.Logs = s.snapshot.Logs[len(s.snapshot.Logs)-1000:]
	}
	_ = s.saveLocked()
}
func (s *Store) Snapshot() Snapshot { s.mu.RLock(); defer s.mu.RUnlock(); return s.snapshot }
func (s *Store) Recent(limit int) []Log {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.snapshot.Logs) {
		limit = len(s.snapshot.Logs)
	}
	logs := make([]Log, limit)
	copy(logs, s.snapshot.Logs[len(s.snapshot.Logs)-limit:])
	for left, right := 0, len(logs)-1; left < right; left, right = left+1, right-1 {
		logs[left], logs[right] = logs[right], logs[left]
	}
	return logs
}
func (s *Store) Providers() []map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	providers := []map[string]string{}
	for _, log := range s.snapshot.Logs {
		if log.Provider == "" || seen[log.Provider] {
			continue
		}
		seen[log.Provider] = true
		providers = append(providers, map[string]string{"id": log.Provider, "name": log.Provider})
	}
	sort.Slice(providers, func(left, right int) bool { return providers[left]["id"] < providers[right]["id"] })
	return providers
}
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
