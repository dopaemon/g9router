package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocaleAPI(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/locale", strings.NewReader(`{"locale":"vi"}`)))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Set-Cookie") == "" {
		t.Fatalf("status=%d headers=%v body=%s", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}
