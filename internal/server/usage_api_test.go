package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"g9router/internal/providers"
	"g9router/internal/usage"
)

func TestUsageLogEndpoints(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	app.usage.AddLog(1, 0, 0, 0, usage.Log{Provider: "openai", Timestamp: time.Now().UTC().Format(time.RFC3339Nano)})
	logs := httptest.NewRecorder()
	app.Handler().ServeHTTP(logs, httptest.NewRequest(http.MethodGet, "/api/usage/logs", nil))
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), `"provider":"openai"`) {
		t.Fatalf("status=%d body=%s", logs.Code, logs.Body.String())
	}
	providers := httptest.NewRecorder()
	app.Handler().ServeHTTP(providers, httptest.NewRequest(http.MethodGet, "/api/usage/providers", nil))
	if providers.Code != http.StatusOK || !strings.Contains(providers.Body.String(), `"id":"openai"`) {
		t.Fatalf("status=%d body=%s", providers.Code, providers.Body.String())
	}
}

func TestCodexResetCreditsRejectsNonCodexConnection(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "openai", APIKey: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/usage/openai/codex-reset-credits", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "only available for Codex") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
