package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Credential struct {
	ID, Provider, AccessToken, RefreshToken, TokenURL, ClientID string
	ExpiresAt                                                   int64 `json:"expiresAt"`
	Scope                                                       string
}
type Manager struct {
	mu     sync.RWMutex
	path   string
	items  map[string]Credential
	client *http.Client
}

func New(path string) *Manager {
	manager := &Manager{path: path, items: map[string]Credential{}, client: &http.Client{Timeout: 30 * time.Second}}
	_ = manager.load()
	return manager
}
func (m *Manager) List() []Credential {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Credential, 0, len(m.items))
	for _, item := range m.items {
		item.AccessToken = ""
		item.RefreshToken = ""
		result = append(result, item)
	}
	return result
}
func (m *Manager) Upsert(item Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[item.ID] = item
	return m.save()
}
func (m *Manager) Get(id string) (Credential, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.items[id]
	return item, ok
}
func (m *Manager) Refresh(ctx context.Context, id string) (Credential, error) {
	m.mu.RLock()
	item, ok := m.items[id]
	m.mu.RUnlock()
	if !ok {
		return Credential{}, fmt.Errorf("credential %q not found", id)
	}
	if item.RefreshToken == "" || item.TokenURL == "" {
		return item, fmt.Errorf("credential %q has no refresh configuration", id)
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {item.RefreshToken}, "client_id": {item.ClientID}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, item.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return item, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := m.client.Do(request)
	if err != nil {
		return item, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return item, fmt.Errorf("token refresh status %s", response.Status)
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return item, err
	}
	if payload.AccessToken == "" {
		return item, fmt.Errorf("token response missing access_token")
	}
	item.AccessToken = payload.AccessToken
	if payload.RefreshToken != "" {
		item.RefreshToken = payload.RefreshToken
	}
	if payload.ExpiresIn > 0 {
		item.ExpiresAt = time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UnixMilli()
	}
	if err := m.Upsert(item); err != nil {
		return item, err
	}
	return item, nil
}
func (m *Manager) load() error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &m.items)
}
func (m *Manager) save() error {
	data, err := json.MarshalIndent(m.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0600)
}
func (c Credential) ExpiringSoon(now time.Time) bool {
	return c.ExpiresAt > 0 && c.ExpiresAt-now.UnixMilli() < 5*60*1000
}
