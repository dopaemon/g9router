package oauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"g9router/internal/db"
)

type Credential struct {
	ID, Provider, AccessToken, RefreshToken, TokenURL, ClientID string
	ExpiresAt                                                   int64 `json:"expiresAt"`
	Scope                                                       string
	ProviderSpecificData                                        map[string]any `json:"providerSpecificData,omitempty"`
}
type Manager struct {
	mu       sync.RWMutex
	path     string
	items    map[string]Credential
	client   *http.Client
	database *sql.DB
}

func New(path string) *Manager {
	manager := &Manager{path: path, items: map[string]Credential{}, client: &http.Client{Timeout: 30 * time.Second}}
	if strings.HasSuffix(path, ".db") {
		if database, err := db.Open(path); err == nil {
			manager.database = database
			_ = manager.loadDB()
			return manager
		}
	}
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

func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = map[string]Credential{}
	if m.database != nil {
		return m.loadDB()
	}
	return m.load()
}
func (m *Manager) Upsert(item Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[item.ID] = item
	if m.database != nil {
		return m.saveDB(item)
	}
	return m.save()
}
func (m *Manager) Get(id string) (Credential, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.items[id]
	return item, ok
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return nil
	}
	delete(m.items, id)
	if m.database != nil {
		_, err := m.database.Exec(`DELETE FROM oauth_credentials WHERE id = ?`, id)
		return err
	}
	return m.save()
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
	if item.Provider == "kiro" {
		return m.refreshKiro(ctx, item)
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
		AccessToken       string `json:"access_token"`
		RefreshToken      string `json:"refresh_token"`
		ExpiresIn         int64  `json:"expires_in"`
		AccessTokenCamel  string `json:"accessToken"`
		RefreshTokenCamel string `json:"refreshToken"`
		ExpiresInCamel    int64  `json:"expiresIn"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return item, err
	}
	if payload.AccessToken == "" {
		payload.AccessToken = payload.AccessTokenCamel
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = payload.RefreshTokenCamel
	}
	if payload.ExpiresIn == 0 {
		payload.ExpiresIn = payload.ExpiresInCamel
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

func (m *Manager) refreshKiro(ctx context.Context, item Credential) (Credential, error) {
	data := item.ProviderSpecificData
	clientID, _ := data["clientId"].(string)
	clientSecret, _ := data["clientSecret"].(string)
	region, _ := data["region"].(string)
	authMethod, _ := data["authMethod"].(string)
	if authMethod == "external_idp" {
		endpoint, _ := data["tokenEndpoint"].(string)
		scope, _ := data["scope"].(string)
		form := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {item.RefreshToken}, "scope": {scope}}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return item, err
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Accept", "application/json")
		response, err := m.client.Do(request)
		if err != nil {
			return item, err
		}
		defer response.Body.Close()
		if response.StatusCode >= 300 {
			return item, fmt.Errorf("token refresh status %s", response.Status)
		}
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			return item, err
		}
		accessToken := stringFrom(payload, "access_token", "accessToken")
		if accessToken == "" {
			return item, fmt.Errorf("token response missing access token")
		}
		item.AccessToken = accessToken
		if refreshed := stringFrom(payload, "refresh_token", "refreshToken"); refreshed != "" {
			item.RefreshToken = refreshed
		}
		if expires := numberFrom(payload, "expires_in", "expiresIn"); expires > 0 {
			item.ExpiresAt = time.Now().Add(time.Duration(expires) * time.Second).UnixMilli()
		}
		return item, m.Upsert(item)
	}
	var endpoint, contentType string
	var body []byte
	if clientID != "" && clientSecret != "" {
		endpoint = "https://oidc.us-east-1.amazonaws.com/token"
		if region != "" {
			endpoint = "https://oidc." + region + ".amazonaws.com/token"
		}
		payload := map[string]string{"clientId": clientID, "clientSecret": clientSecret, "refreshToken": item.RefreshToken, "grantType": "refresh_token"}
		encoded, _ := json.Marshal(payload)
		body, contentType = encoded, "application/json"
	} else {
		endpoint = item.TokenURL
		encoded, _ := json.Marshal(map[string]string{"refreshToken": item.RefreshToken})
		body, contentType = encoded, "application/json"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return item, err
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "kiro-cli/1.0.0")
	response, err := m.client.Do(request)
	if err != nil {
		return item, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return item, fmt.Errorf("token refresh status %s", response.Status)
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return item, err
	}
	accessToken := stringFrom(payload, "accessToken", "access_token")
	if accessToken == "" {
		return item, fmt.Errorf("token response missing access token")
	}
	item.AccessToken = accessToken
	if refreshed := stringFrom(payload, "refreshToken", "refresh_token"); refreshed != "" {
		item.RefreshToken = refreshed
	}
	if expires := numberFrom(payload, "expiresIn", "expires_in"); expires > 0 {
		item.ExpiresAt = time.Now().Add(time.Duration(expires) * time.Second).UnixMilli()
	}
	return item, m.Upsert(item)
}

func stringFrom(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}
func numberFrom(payload map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := payload[key].(float64); ok {
			return int64(value)
		}
	}
	return 0
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
func (m *Manager) loadDB() error {
	rows, err := m.database.Query(`SELECT id,payload FROM oauth_credentials`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, payload string
		if err := rows.Scan(&id, &payload); err != nil {
			return err
		}
		var item Credential
		if json.Unmarshal([]byte(payload), &item) == nil {
			m.items[id] = item
		}
	}
	return rows.Err()
}
func (m *Manager) saveDB(item Credential) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = m.database.Exec(`INSERT INTO oauth_credentials(id,payload,updated_at) VALUES(?,?,unixepoch()) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, item.ID, string(payload))
	return err
}
func (c Credential) ExpiringSoon(now time.Time) bool {
	return c.ExpiresAt > 0 && c.ExpiresAt-now.UnixMilli() < 5*60*1000
}
