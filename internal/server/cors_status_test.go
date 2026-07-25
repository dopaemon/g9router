package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceCompatiblePreflightStatus(t *testing.T) {
	app := New(Options{DatabasePath: t.TempDir() + "/test.db"})
	for _, path := range []string{"/v1/messages/count_tokens", "/v1/api/chat", "/v1/audio/voices", "/v1beta/models/gemini/test:generateContent"} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodOptions, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s: status=%d, want %d", path, response.Code, http.StatusOK)
		}
	}
}
