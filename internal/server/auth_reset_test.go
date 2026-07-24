package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResetPasswordAPI(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "{\"success\":true}\n" {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
