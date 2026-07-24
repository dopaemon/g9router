package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTunnelStatus(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tunnel/status", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"tunnel"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
