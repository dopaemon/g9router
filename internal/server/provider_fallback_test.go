package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"g9router/internal/providers"
)

func TestChatProviderFallback(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "temporary", http.StatusBadGateway) }))
	defer failing.Close()
	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fallback","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer working.Close()
	path := os.TempDir() + "/g9router-fallback-test.json"
	defer os.Remove(path)
	app := New(Options{ProviderPath: path, OAuthPath: path + ".oauth"})
	if err := app.store.Upsert(providers.Provider{ID: "demo", BaseURL: failing.URL + "/v1/chat/completions", APIType: "openai", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := app.store.Upsert(providers.Provider{ID: "backup", BaseURL: working.URL + "/v1/chat/completions", APIType: "openai", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"demo/model","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
