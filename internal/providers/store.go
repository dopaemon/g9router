package providers

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type Provider struct {
	ID, Name, BaseURL, APIKey, APIType string
	Enabled                            bool `json:"enabled"`
}
type Store struct {
	mu    sync.RWMutex
	path  string
	items []Provider
}

func (s *Store) Resolve(model string) []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matched, fallback []Provider
	for _, item := range s.items {
		if !item.Enabled {
			continue
		}
		if strings.HasPrefix(model, item.ID+"/") {
			matched = append(matched, item)
		} else {
			fallback = append(fallback, item)
		}
	}
	return append(matched, fallback...)
}

func New(path string) *Store { store := &Store{path: path}; _ = store.load(); return store }
func (s *Store) List() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Provider, len(s.items))
	copy(result, s.items)
	for i := range result {
		result[i].APIKey = ""
	}
	return result
}
func (s *Store) Upsert(provider Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == provider.ID {
			s.items[i] = provider
			return s.save()
		}
	}
	s.items = append(s.items, provider)
	return s.save()
}
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			break
		}
	}
	return s.save()
}
func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.items)
}
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
