package settings

import (
	"database/sql"
	"encoding/json"
	"sync"
)

type Store struct {
	mu       sync.RWMutex
	database *sql.DB
	values   map[string]any
}

func New(database *sql.DB) *Store {
	store := &Store{database: database, values: map[string]any{}}
	store.load()
	return store
}
func (s *Store) Get() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[string]any{}
	for key, value := range s.values {
		if key != "password" && key != "apiKey" {
			result[key] = value
		}
	}
	return result
}
func (s *Store) Update(values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range values {
		s.values[key] = value
	}
	payload, err := json.Marshal(s.values)
	if err != nil {
		return err
	}
	_, err = s.database.Exec(`INSERT INTO settings(id,payload,updated_at) VALUES(1,?,unixepoch()) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, string(payload))
	return err
}

func (s *Store) ModelAliases() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[string]string{}
	if values, ok := s.values["modelAliases"].(map[string]any); ok {
		for alias, model := range values {
			if value, ok := model.(string); ok {
				result[alias] = value
			}
		}
	}
	return result
}

func (s *Store) SetModelAlias(alias, model string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	aliases, _ := s.values["modelAliases"].(map[string]any)
	if aliases == nil {
		aliases = map[string]any{}
		s.values["modelAliases"] = aliases
	}
	aliases[alias] = model
	return s.saveLocked()
}

func (s *Store) DeleteModelAlias(alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if aliases, ok := s.values["modelAliases"].(map[string]any); ok {
		delete(aliases, alias)
	}
	return s.saveLocked()
}

func (s *Store) CustomModels() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	models, _ := s.values["customModels"].([]any)
	result := make([]map[string]any, 0, len(models))
	for _, raw := range models {
		if model, ok := raw.(map[string]any); ok {
			result = append(result, model)
		}
	}
	return result
}

func (s *Store) AddCustomModel(model map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	models, _ := s.values["customModels"].([]any)
	models = append(models, model)
	s.values["customModels"] = models
	return model, s.saveLocked()
}

func (s *Store) DeleteCustomModel(providerAlias, id, kind string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	models, _ := s.values["customModels"].([]any)
	filtered := models[:0]
	for _, raw := range models {
		model, _ := raw.(map[string]any)
		if model["providerAlias"] == providerAlias && model["id"] == id && model["type"] == kind {
			continue
		}
		filtered = append(filtered, raw)
	}
	s.values["customModels"] = filtered
	return s.saveLocked()
}

func (s *Store) DisabledModels() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := map[string][]string{}
	all, _ := s.values["disabledModels"].(map[string]any)
	for provider, raw := range all {
		ids, _ := raw.([]any)
		for _, value := range ids {
			if id, ok := value.(string); ok {
				result[provider] = append(result[provider], id)
			}
		}
	}
	return result
}

func (s *Store) SetDisabledModels(provider string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, _ := s.values["disabledModels"].(map[string]any)
	if all == nil {
		all = map[string]any{}
		s.values["disabledModels"] = all
	}
	values := make([]any, len(ids))
	for index, id := range ids {
		values[index] = id
	}
	all[provider] = values
	return s.saveLocked()
}

func (s *Store) EnableModels(provider string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, _ := s.values["disabledModels"].(map[string]any)
	if all == nil {
		return nil
	}
	if len(ids) == 0 {
		delete(all, provider)
		return s.saveLocked()
	}
	remove := map[string]bool{}
	for _, id := range ids {
		remove[id] = true
	}
	current, _ := all[provider].([]any)
	kept := current[:0]
	for _, raw := range current {
		if id, ok := raw.(string); !ok || !remove[id] {
			kept = append(kept, raw)
		}
	}
	all[provider] = kept
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	payload, err := json.Marshal(s.values)
	if err != nil {
		return err
	}
	_, err = s.database.Exec(`INSERT INTO settings(id,payload,updated_at) VALUES(1,?,unixepoch()) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, string(payload))
	return err
}
func (s *Store) load() {
	var payload string
	if s.database.QueryRow(`SELECT payload FROM settings WHERE id=1`).Scan(&payload) == nil {
		_ = json.Unmarshal([]byte(payload), &s.values)
	}
}
