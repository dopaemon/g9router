package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchValidation(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/web/fetch", strings.NewReader(`{"provider":"jina-reader","url":"file:///etc/passwd"}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid URL") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
