package providernodes

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

type Node struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Prefix  string `json:"prefix"`
	APIType string `json:"apiType,omitempty"`
	BaseURL string `json:"baseUrl"`
	Name    string `json:"name"`
}
type Store struct {
	mu    sync.Mutex
	path  string
	items []Node
}

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
func (s *Store) List() []Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Node, len(s.items))
	copy(out, s.items)
	return out
}
func (s *Store) Get(id string) (Node, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return Node{}, false
}
func (s *Store) Create(item Node) (Node, error) {
	if item.Name == "" || item.Prefix == "" {
		return Node{}, fmt.Errorf("name and prefix are required")
	}
	if item.Type == "" {
		item.Type = "openai-compatible"
	}
	if item.Type == "openai-compatible" && item.APIType != "chat" && item.APIType != "responses" {
		return Node{}, fmt.Errorf("invalid OpenAI compatible API type")
	}
	item.BaseURL = sanitize(item.BaseURL, item.Type)
	item.ID = fmt.Sprintf("%s-%d", strings.ReplaceAll(item.Type, "-", "_"), len(s.items)+1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, item)
	return item, s.saveLocked()
}
func (s *Store) Update(id string, item Node) (Node, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.items {
		if s.items[index].ID == id {
			item.ID = id
			item.Type = s.items[index].Type
			if item.Name == "" || item.Prefix == "" || item.BaseURL == "" {
				return Node{}, false, fmt.Errorf("name, prefix and baseUrl are required")
			}
			if item.Type == "openai-compatible" && item.APIType != "chat" && item.APIType != "responses" {
				return Node{}, false, fmt.Errorf("invalid OpenAI compatible API type")
			}
			item.BaseURL = sanitize(item.BaseURL, item.Type)
			s.items[index] = item
			return item, true, s.saveLocked()
		}
	}
	return Node{}, false, nil
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
func sanitize(value, kind string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if kind == "anthropic-compatible" {
		value = strings.TrimSuffix(value, "/messages")
	}
	if kind == "custom-embedding" {
		value = strings.TrimSuffix(value, "/embeddings")
	}
	return value
}
