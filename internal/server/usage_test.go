package server

import (
	"g9router/internal/providers"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsageResourceRejectsUnsupportedAPIKeyProvider(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	if err := app.store.Upsert(providers.Provider{ID: "openai", APIKey: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/usage/openai", nil))
	if response.Code != http.StatusOK || response.Body.String() != "{\"message\":\"Usage not available for this connection\"}\n" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
