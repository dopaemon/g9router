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
