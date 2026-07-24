package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOIDCTestValidation(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/oidc/test", strings.NewReader(`{"clientId":"client"}`)))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "Issuer URL is required") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
