package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUsageDetailsValidation(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	bad := httptest.NewRecorder()
	app.Handler().ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/usage/chart?period=bad", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("chart status=%d body=%s", bad.Code, bad.Body.String())
	}
	details := httptest.NewRecorder()
	app.Handler().ServeHTTP(details, httptest.NewRequest(http.MethodGet, "/api/usage/request-details?page=0", nil))
	if details.Code != http.StatusBadRequest || !strings.Contains(details.Body.String(), "Page must be") {
		t.Fatalf("details status=%d body=%s", details.Code, details.Body.String())
	}
}
