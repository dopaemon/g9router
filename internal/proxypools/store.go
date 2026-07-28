package proxypools

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"g9router/internal/db"
)

type Pool struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	ProxyURL             string `json:"proxyUrl"`
	NoProxy              string `json:"noProxy,omitempty"`
	IsActive             bool   `json:"isActive"`
	StrictProxy          bool   `json:"strictProxy"`
	Type                 string `json:"type"`
	TestStatus           string `json:"testStatus,omitempty"`
	LastTestedAt         string `json:"lastTestedAt,omitempty"`
	LastError            string `json:"lastError,omitempty"`
	BoundConnectionCount int    `json:"boundConnectionCount,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	items []Pool
}

func New(path string) *Store {
	s := &Store{path: path}
	_ = db.ReadJSON(path, &s.items)
	return s
}

func (s *Store) List(active *bool) []Pool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Pool, 0, len(s.items))
	for _, item := range s.items {
		if active != nil && item.IsActive != *active {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (s *Store) Get(id string) (Pool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return Pool{}, false
}

func (s *Store) Create(item Pool) (Pool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item.ID = newID()
	s.items = append(s.items, item)
	return item, s.saveLocked()
}

func (s *Store) Update(id string, updates map[string]any) (Pool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.items {
		if s.items[index].ID != id {
			continue
		}
		item := &s.items[index]
		if value, ok := updates["name"].(string); ok {
			item.Name = value
		}
		if value, ok := updates["proxyUrl"].(string); ok {
			item.ProxyURL = value
		}
		if value, ok := updates["noProxy"].(string); ok {
			item.NoProxy = value
		}
		if value, ok := updates["isActive"].(bool); ok {
			item.IsActive = value
		}
		if value, ok := updates["strictProxy"].(bool); ok {
			item.StrictProxy = value
		}
		if value, ok := updates["type"].(string); ok {
			item.Type = value
		}
		if value, ok := updates["testStatus"].(string); ok {
			item.TestStatus = value
		}
		if value, ok := updates["lastTestedAt"].(string); ok {
			item.LastTestedAt = value
		}
		if value, ok := updates["lastError"].(string); ok {
			item.LastError = value
		}
		if value, ok := updates["lastError"]; ok && value == nil {
			item.LastError = ""
		}
		return *item, true, s.saveLocked()
	}
	return Pool{}, false, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.items {
		if s.items[index].ID == id {
			s.items = append(s.items[:index], s.items[index+1:]...)
			break
		}
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	return db.WriteJSON(s.path, s.items)
}

func newID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
}
