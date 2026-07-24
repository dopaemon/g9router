package usage

import (
	"encoding/json"
	"os"
	"sync"
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
}

func New(path string) *Store {
	store := &Store{path: path}
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
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(s.snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
