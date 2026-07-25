package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOAuthProxyContract(t *testing.T) {
	app := New(Options{DatabasePath: t.TempDir() + "/test.db"})
	for _, path := range []string{
		"/api/oauth/codex/start-proxy",
		"/api/oauth/xai/start-proxy",
	} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "app_port") {
			t.Fatalf("%s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/oauth/codex/poll-status?state=missing", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "unknown") {
		t.Fatalf("poll: status=%d body=%s", response.Code, response.Body.String())
	}
}
