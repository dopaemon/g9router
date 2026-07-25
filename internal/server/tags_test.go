package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTagsAPI(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tags", nil))
	if recorder.Code != http.StatusOK || !strings.HasPrefix(recorder.Body.String(), "[") || !strings.Contains(recorder.Body.String(), `"llama3.2"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
