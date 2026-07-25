package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPricingValidation(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	bad := httptest.NewRecorder()
	app.Handler().ServeHTTP(bad, httptest.NewRequest(http.MethodPatch, "/api/pricing", strings.NewReader(`{"openai":{"gpt-5":{"input":-1}}}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", bad.Code, bad.Body.String())
	}
	good := httptest.NewRecorder()
	app.Handler().ServeHTTP(good, httptest.NewRequest(http.MethodPatch, "/api/pricing", strings.NewReader(`{"openai":{"gpt-5":{"input":1.25,"output":10}}}`)))
	if good.Code != http.StatusOK || !strings.Contains(good.Body.String(), `"input":1.25`) {
		t.Fatalf("status=%d body=%s", good.Code, good.Body.String())
	}
}

func TestPricingDefaultsAndMerge(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	defaults := httptest.NewRecorder()
	app.Handler().ServeHTTP(defaults, httptest.NewRequest(http.MethodGet, "/api/pricing/defaults", nil))
	if defaults.Code != http.StatusOK || !strings.Contains(defaults.Body.String(), `"gh"`) {
		t.Fatalf("defaults status=%d body=%s", defaults.Code, defaults.Body.String())
	}
	update := httptest.NewRecorder()
	app.Handler().ServeHTTP(update, httptest.NewRequest(http.MethodPatch, "/api/pricing", strings.NewReader(`{"custom":{"model":{"input":1,"output":2}}}`)))
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"gh"`) || !strings.Contains(update.Body.String(), `"custom"`) {
		t.Fatalf("merged status=%d body=%s", update.Code, update.Body.String())
	}
	reset := httptest.NewRecorder()
	app.Handler().ServeHTTP(reset, httptest.NewRequest(http.MethodDelete, "/api/pricing?provider=custom", nil))
	if reset.Code != http.StatusOK || strings.Contains(reset.Body.String(), `"custom"`) {
		t.Fatalf("reset status=%d body=%s", reset.Code, reset.Body.String())
	}
}
