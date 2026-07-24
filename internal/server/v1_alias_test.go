package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV1AliasReturnsModels(t *testing.T) {
	app := New(Options{})
	for _, path := range []string{"/v1", "/api/v1"} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data"`) {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
