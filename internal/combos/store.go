package combos

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"g9router/internal/db"
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
	return db.ReadJSON(s.path, &s.items)
}
func (s *Store) saveLocked() error {
	return db.WriteJSON(s.path, s.items)
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
	item := Combo{ID: fmt.Sprintf("combo-%d", s.nextID()), Name: name, Models: models, Kind: kind}
	s.items = append(s.items, item)
	return item, s.saveLocked()
}

func (s *Store) nextID() int {
	next := 1
	for _, item := range s.items {
		value, err := strconv.Atoi(strings.TrimPrefix(item.ID, "combo-"))
		if err == nil && value >= next {
			next = value + 1
		}
	}
	return next
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
