package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAudioVoicesRejectsUnknownProvider(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/audio/voices?provider=unknown", nil))
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_request_error") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
