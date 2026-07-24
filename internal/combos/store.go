package combos

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sync"
)

type Combo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Models []any  `json:"models"`
	Kind   string `json:"kind,omitempty"`
}
type Store struct {
	mu    sync.Mutex
	path  string
	items []Combo
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func New(path string) *Store { store := &Store{path: path}; _ = store.load(); return store }
func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
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
	for index := len(path) - 1; index >= 0; index-- {
		if path[index] == '/' {
			if index == 0 {
				return "/"
			}
			return path[:index]
		}
	}
	return "."
}
func (s *Store) List() []Combo {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Combo, len(s.items))
	copy(result, s.items)
	return result
}
func (s *Store) Get(id string) (Combo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return Combo{}, false
}
func (s *Store) Create(name string, models []any, kind string) (Combo, error) {
	if !validName.MatchString(name) {
		return Combo{}, fmt.Errorf("invalid combo name")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.Name == name {
			return Combo{}, fmt.Errorf("combo name already exists")
		}
	}
	item := Combo{ID: fmt.Sprintf("combo-%d", len(s.items)+1), Name: name, Models: models, Kind: kind}
	s.items = append(s.items, item)
	return item, s.saveLocked()
}
func (s *Store) Update(id string, value Combo) (Combo, bool, error) {
	if value.Name != "" && !validName.MatchString(value.Name) {
		return Combo{}, false, fmt.Errorf("invalid combo name")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.items {
		if s.items[index].ID == id {
			for _, item := range s.items {
				if item.ID != id && value.Name != "" && item.Name == value.Name {
					return Combo{}, false, fmt.Errorf("combo name already exists")
				}
			}
			if value.Name != "" {
				s.items[index].Name = value.Name
			}
			if value.Models != nil {
				s.items[index].Models = value.Models
			}
			if value.Kind != "" {
				s.items[index].Kind = value.Kind
			}
			if err := s.saveLocked(); err != nil {
				return Combo{}, false, err
			}
			return s.items[index], true, nil
		}
	}
	return Combo{}, false, nil
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
