package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPXPIPEStatusAndHealth(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	status := httptest.NewRecorder()
	app.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/pxpipe/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"installed":false`) {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}
	health := httptest.NewRecorder()
	app.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodPost, "/api/pxpipe/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"healthy":false`) {
		t.Fatalf("health=%d body=%s", health.Code, health.Body.String())
	}
}
