package providers

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type Provider struct {
	ID, Name, BaseURL, APIKey, APIType string
	Enabled                            bool      `json:"enabled"`
	Accounts                           []Account `json:"accounts,omitempty"`
}

type Account struct {
	ID, APIKey    string
	Enabled       bool  `json:"enabled"`
	RequestsLimit int64 `json:"requestsLimit,omitempty"`
	RequestsUsed  int64 `json:"requestsUsed,omitempty"`
}
type Store struct {
	mu    sync.RWMutex
	path  string
	items []Provider
	next  map[string]int
}

func (s *Store) Resolve(model string) []Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	var matched, fallback []Provider
	for index := range s.items {
		item := s.items[index]
		if !item.Enabled {
			continue
		}
		selected := item
		if len(item.Accounts) > 0 {
			selected.APIKey = s.accountKeyLocked(index)
			selected.Accounts = s.items[index].Accounts
		}
		if strings.HasPrefix(model, item.ID+"/") {
			matched = append(matched, selected)
		} else {
			fallback = append(fallback, selected)
		}
	}
	return append(matched, fallback...)
}

func New(path string) *Store {
	store := &Store{path: path, next: map[string]int{}}
	_ = store.load()
	return store
}
func (s *Store) List() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Provider, len(s.items))
	copy(result, s.items)
	for i := range result {
		result[i].APIKey = ""
		for j := range result[i].Accounts {
			result[i].Accounts[j].APIKey = ""
		}
	}
	return result
}

func (s *Store) accountKeyLocked(providerIndex int) string {
	provider := &s.items[providerIndex]
	accounts := provider.Accounts
	if len(accounts) == 0 {
		return provider.APIKey
	}
	start := s.next[provider.ID] % len(accounts)
	for offset := 0; offset < len(accounts); offset++ {
		index := (start + offset) % len(accounts)
		account := accounts[index]
		if !account.Enabled || (account.RequestsLimit > 0 && account.RequestsUsed >= account.RequestsLimit) {
			continue
		}
		provider.Accounts[index].RequestsUsed++
		s.next[provider.ID] = index + 1
		return account.APIKey
	}
	return ""
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
