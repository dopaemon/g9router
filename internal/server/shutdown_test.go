package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShutdownRequiresSecret(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/shutdown", nil))
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "Unauthorized") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
