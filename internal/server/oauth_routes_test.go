package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKimiCodingOAuthRoutesAreRegistered(t *testing.T) {
	app := New(Options{DatabasePath: t.TempDir() + "/test.db"})
	for _, path := range []string{"/api/oauth/kimi-coding/device-code", "/api/oauth/kimi-coding/poll"} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPut, path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: status=%d", path, response.Code)
		}
	}
}
