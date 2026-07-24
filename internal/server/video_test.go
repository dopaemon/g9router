package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"g9router/internal/providers"
)

func TestVideoProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/videos/generations" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing authorization")
		}
		fmt.Fprint(w, `{"request_id":"vid-1","status":"pending"}`)
	}))
	defer upstream.Close()
	path := t.TempDir() + "/providers.json"
	app := New(Options{ProviderPath: path, OAuthPath: os.TempDir() + "/video-oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "xai", BaseURL: upstream.URL + "/v1/chat/completions", APIKey: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(`{"model":"grok-imagine-video","prompt":"cat"}`))
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "vid-1") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestVideoProxyAPIPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/videos/generations" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"request_id":"vid-api"}`)
	}))
	defer upstream.Close()
	path := t.TempDir() + "/providers.json"
	app := New(Options{ProviderPath: path, OAuthPath: os.TempDir() + "/video-api-oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "xai", BaseURL: upstream.URL + "/v1/chat/completions", APIKey: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/videos/generations", strings.NewReader(`{}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "vid-api") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
