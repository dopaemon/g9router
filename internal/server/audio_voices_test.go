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

func TestAudioVoicesAcceptsLocalDevice(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/audio/voices?provider=local-device", nil))
	if recorder.Code == http.StatusBadGateway || strings.Contains(recorder.Body.String(), "not implemented") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicAudioVoicesRejectsInternalOnlyProviders(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	for _, provider := range []string{"minimax", "gemini"} {
		recorder := httptest.NewRecorder()
		app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/audio/voices?provider="+provider, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_request_error") {
			t.Fatalf("provider=%s status=%d body=%s", provider, recorder.Code, recorder.Body.String())
		}
	}
}

func TestLocalDeviceVoicesExposeGroupedCatalog(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json", DatabasePath: t.TempDir() + "/state.db"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/media-providers/tts/voices?provider=local-device", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"byLang"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
