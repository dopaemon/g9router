package server

import (
	"g9router/internal/providers"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestProviderModelsEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"demo-model"}]}`)
	}))
	defer upstream.Close()
	app := New(Options{ProviderPath: os.TempDir() + "/models-test.json"})
	if err := app.store.Upsert(providers.Provider{ID: "demo", BaseURL: upstream.URL, APIKey: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/api/providers/demo/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.StatusCode)
	}
}

func TestProviderClientReturnsPaginatedConnections(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	for _, provider := range []providers.Provider{
		{ID: "alpha", Enabled: true},
		{ID: "beta", Enabled: true},
	} {
		if err := app.store.Upsert(provider); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/providers/client?page=2&pageSize=1&sort=provider", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"page":2`) || !strings.Contains(response.Body.String(), `"id":"beta"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProviderClientFiltersInactiveConnections(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	for _, provider := range []providers.Provider{{ID: "active", Enabled: true}, {ID: "inactive", Enabled: false}} {
		if err := app.store.Upsert(provider); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/providers/client?accountStatus=inactive", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"inactive"`) || strings.Contains(response.Body.String(), `"id":"active"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProviderClientRedactsUnsafeProviderData(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "demo", Enabled: true, ProviderSpecificData: map[string]any{"region": "ams", "apiKey": "secret", "refreshToken": "refresh"}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/providers/client", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"region":"ams"`) || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "refresh") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProviderTestModelsDiscoversCustomModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"custom-model","name":"Custom Model"}]}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-test","choices":[]}`)
	}))
	defer upstream.Close()
	app := New(Options{ProviderPath: os.TempDir() + "/test-models-discovery.json"})
	if err := app.store.Upsert(providers.Provider{ID: "custom", BaseURL: upstream.URL, APIKey: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/custom/test-models", nil)
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"modelId":"custom-model"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProviderResourceCRUD(t *testing.T) {
	app := New(Options{ProviderPath: os.TempDir() + "/resource-test.json"})
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/providers", strings.NewReader(`{"id":"demo","baseURL":"http://demo","enabled":true}`))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatal(response.StatusCode)
	}
	response.Body.Close()
	response, err = http.Get(server.URL + "/api/providers/demo")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(body), `"connection"`) {
		t.Fatalf("unexpected detail response: %s", body)
	}
}

func TestProviderAPIListsSafeConnections(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	if err := app.store.Upsert(providers.Provider{ID: "demo", BaseURL: "http://demo", APIKey: "secret", Enabled: true, ProviderSpecificData: map[string]any{"region": "ams", "refreshToken": "refresh"}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/providers", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"connections"`) || !strings.Contains(body, `"region":"ams"`) || strings.Contains(body, "secret") || strings.Contains(body, "refresh") {
		t.Fatalf("status=%d body=%s", response.Code, body)
	}
}

func TestProviderAccountsCRUD(t *testing.T) {
	app := New(Options{ProviderPath: os.TempDir() + "/account-resource-test.json"})
	if err := app.store.Upsert(providers.Provider{ID: "demo", BaseURL: "http://demo", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/providers/demo/accounts", strings.NewReader(`{"apiKey":"secret","name":"primary"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatal(err, response.StatusCode)
	}
	response.Body.Close()
	response, err = http.Get(server.URL + "/api/providers/demo/accounts")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal(err, response.StatusCode)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(body), "secret") {
		t.Fatal("account API key leaked")
	}
}

func TestModelSettingsAPI(t *testing.T) {
	app := New(Options{ProviderPath: os.TempDir() + "/model-settings-test.json"})
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPut, server.URL+"/api/models/alias", strings.NewReader(`{"model":"gpt-x","alias":"fast"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal(err, response.StatusCode)
	}
	response.Body.Close()
	response, err = http.Get(server.URL + "/api/models/alias")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.StatusCode)
	}
	request, _ = http.NewRequest(http.MethodPost, server.URL+"/api/models/disabled", strings.NewReader(`{"providerAlias":"openai","ids":["gpt-x"]}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal(err, response.StatusCode)
	}
	response.Body.Close()
}

func TestSettingsAPIHidesProtectedFields(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", DatabasePath: t.TempDir() + "/settings.db"})
	if err := app.settings.Update(map[string]any{"password": "hash", "oidcClientSecret": "oidc-secret", "mitmSudoEncrypted": "sudo-secret"}); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRecorder()
	app.settingsAPI(get, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if get.Code != http.StatusOK || get.Header().Get("Cache-Control") != "no-store" || strings.Contains(get.Body.String(), "oidc-secret") || strings.Contains(get.Body.String(), "sudo-secret") || !strings.Contains(get.Body.String(), `"hasPassword":true`) {
		t.Fatalf("status=%d headers=%v body=%s", get.Code, get.Header(), get.Body.String())
	}
	patch := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(`{"mitmSudoEncrypted":"attacker","oidcClientSecret":""}`))
	app.settingsAPI(patch, request)
	if patch.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", patch.Code, patch.Body.String())
	}
	if value, ok := app.settings.Secret("mitmSudoEncrypted"); !ok || value != "sudo-secret" {
		t.Fatalf("protected setting changed: %q", value)
	}
}

func TestProviderResourcePutMergesPartialConnection(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	if err := app.store.Upsert(providers.Provider{ID: "demo", Name: "Old", BaseURL: "http://old", APIKey: "secret", Enabled: true, ProviderSpecificData: map[string]any{"region": "ams"}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/providers/demo", strings.NewReader(`{"name":"New"}`))
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	provider, ok := app.store.Find("demo")
	if !ok || provider.Name != "New" || provider.BaseURL != "http://old" || provider.APIKey != "secret" || provider.ProviderSpecificData["region"] != "ams" {
		t.Fatalf("partial update lost data: %#v", provider)
	}
}
