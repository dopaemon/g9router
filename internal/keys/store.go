package keys

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Key struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	MachineID string    `json:"machineId"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

type Store struct {
	mu    sync.Mutex
	path  string
	items []Key
}

func New(path string) *Store { store := &Store{path: path}; _ = store.load(); return store }
func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(raw, &s.items)
}
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepathDir(s.path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0600)
}
func filepathDir(path string) string {
	index := len(path) - 1
	for index >= 0 && path[index] != '/' {
		index--
	}
	if index < 0 {
		return "."
	}
	if index == 0 {
		return "/"
	}
	return path[:index]
}
func (s *Store) List() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Key, len(s.items))
	copy(result, s.items)
	return result
}
func (s *Store) Get(id string) (Key, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return Key{}, false
}
func (s *Store) Create(name, machineID string) (Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return Key{}, err
	}
	item := Key{ID: hex.EncodeToString(raw[:8]), Name: name, Key: "sk-9router-" + hex.EncodeToString(raw), MachineID: machineID, IsActive: true, CreatedAt: time.Now().UTC()}
	s.items = append(s.items, item)
	if err := s.saveLocked(); err != nil {
		return Key{}, err
	}
	return item, nil
}
func (s *Store) Update(id string, active bool) (Key, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.items {
		if s.items[index].ID == id {
			s.items[index].IsActive = active
			if err := s.saveLocked(); err != nil {
				return Key{}, false, err
			}
			item := s.items[index]
			return item, true, nil
		}
	}
	return Key{}, false, nil
}
func (s *Store) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.items {
		if s.items[index].ID == id {
			s.items = append(s.items[:index], s.items[index+1:]...)
			return true, s.saveLocked()
		}
	}
	return false, nil
}

func (s *Store) Valid(value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.IsActive && item.Key == value {
			return true
		}
	}
	return false
}

func (s *Store) HasActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.IsActive {
			return true
		}
	}
	return false
}
