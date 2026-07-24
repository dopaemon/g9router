package usage

import "sync"

type Snapshot struct {
	Requests, Errors        int64 `json:"requests"`
	InputBytes, OutputBytes int64 `json:"inputBytes"`
}
type Store struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

func (s *Store) Add(requests, errors, input, output int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Requests += requests
	s.snapshot.Errors += errors
	s.snapshot.InputBytes += input
	s.snapshot.OutputBytes += output
}
func (s *Store) Snapshot() Snapshot { s.mu.RLock(); defer s.mu.RUnlock(); return s.snapshot }
