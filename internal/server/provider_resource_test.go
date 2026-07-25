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

func TestProviderResourceCRUD(t *testing.T) {
	app := New(Options{ProviderPath: os.TempDir() + "/resource-test.json"})
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/providers", strings.NewReader(`{"id":"demo","baseURL":"http://demo","enabled":true}`))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
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
