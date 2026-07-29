package providers

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"sync"

	"g9router/internal/db"
)

type Provider struct {
	ID, Name, BaseURL, APIKey, APIType, OAuthID string
	Enabled                                     bool             `json:"enabled"`
	Accounts                                    []Account        `json:"accounts,omitempty"`
	ProviderSpecificData                        map[string]any   `json:"providerSpecificData,omitempty"`
	ModelLocks                                  map[string]int64 `json:"modelLocks,omitempty"`
	TestStatus                                  string           `json:"testStatus,omitempty"`
	LastError                                   string           `json:"lastError,omitempty"`
}

type Account struct {
	ID            string `json:"id,omitempty"`
	APIKey        string `json:"apiKey,omitempty"`
	OAuthID       string `json:"oauthId,omitempty"`
	Name          string `json:"name,omitempty"`
	Email         string `json:"email,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	Plan          string `json:"plan,omitempty"`
	Enabled       bool   `json:"enabled"`
	RequestsLimit int64  `json:"requestsLimit,omitempty"`
	RequestsUsed  int64  `json:"requestsUsed,omitempty"`
}
type Store struct {
	mu       sync.RWMutex
	path     string
	items    []Provider
	next     map[string]int
	database *sql.DB
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
			selected.APIKey, selected.OAuthID = s.accountKeyLocked(index)
			selected.Accounts = s.items[index].Accounts
		}
		alias := item.ID
		if descriptor, ok := Registry[item.ID]; ok && descriptor.Alias != "" {
			alias = descriptor.Alias
		}
		if strings.HasPrefix(model, item.ID+"/") || strings.HasPrefix(model, alias+"/") {
			matched = append(matched, selected)
		} else {
			fallback = append(fallback, selected)
		}
	}
	return append(matched, fallback...)
}

func (s *Store) Enabled() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []Provider{}
	for _, item := range s.items {
		if item.Enabled {
			result = append(result, cloneProvider(item))
		}
	}
	return result
}

func New(path string) *Store {
	store := &Store{path: path, next: map[string]int{}}
	if strings.HasSuffix(path, ".db") {
		if database, err := db.Open(path); err == nil {
			store.database = database
			_ = store.loadDB()
			return store
		}
	}
	_ = store.load()
	return store
}
func (s *Store) List() []Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Provider, len(s.items))
	for i := range result {
		result[i] = cloneProvider(s.items[i])
		result[i].APIKey = ""
		for j := range result[i].Accounts {
			result[i].Accounts[j].APIKey = ""
		}
	}
	return result
}

func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
	if s.database != nil {
		return s.loadDB()
	}
	return s.load()
}

func (s *Store) Find(id string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, provider := range s.items {
		if provider.ID == id {
			return cloneProvider(provider), true
		}
	}
	return Provider{}, false
}

func cloneProvider(provider Provider) Provider {
	provider.Accounts = append([]Account(nil), provider.Accounts...)
	if provider.ProviderSpecificData != nil {
		data := provider.ProviderSpecificData
		provider.ProviderSpecificData = make(map[string]any, len(data))
		for key, value := range data {
			provider.ProviderSpecificData[key] = value
		}
	}
	if provider.ModelLocks != nil {
		locks := provider.ModelLocks
		provider.ModelLocks = make(map[string]int64, len(locks))
		for key, value := range locks {
			provider.ModelLocks[key] = value
		}
	}
	return provider
}

func (s *Store) accountKeyLocked(providerIndex int) (string, string) {
	provider := &s.items[providerIndex]
	accounts := provider.Accounts
	if len(accounts) == 0 {
		return provider.APIKey, provider.OAuthID
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
		return account.APIKey, account.OAuthID
	}
	return "", ""
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
	if s.database != nil {
		return s.saveDB(provider)
	}
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
	if s.database != nil {
		_, err := s.database.Exec(`DELETE FROM providers WHERE id=?`, id)
		return err
	}
	return s.save()
}

func (s *Store) loadDB() error {
	rows, err := s.database.Query(`SELECT payload FROM providers ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return err
		}
		var provider Provider
		if json.Unmarshal([]byte(payload), &provider) == nil {
			s.items = append(s.items, provider)
		}
	}
	return rows.Err()
}
func (s *Store) saveDB(provider Provider) error {
	payload, err := json.Marshal(provider)
	if err != nil {
		return err
	}
	_, err = s.database.Exec(`INSERT INTO providers(id,payload,updated_at) VALUES(?,?,unixepoch()) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, provider.ID, string(payload))
	return err
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
