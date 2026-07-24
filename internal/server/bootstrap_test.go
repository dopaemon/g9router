package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInitAndVersionAPIs(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	initResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(initResponse, httptest.NewRequest(http.MethodGet, "/api/init", nil))
	if initResponse.Code != http.StatusOK || initResponse.Body.String() != "Initialized" {
		t.Fatalf("init status=%d body=%s", initResponse.Code, initResponse.Body.String())
	}
	versionResponse := httptest.NewRecorder()
	app.Handler().ServeHTTP(versionResponse, httptest.NewRequest(http.MethodGet, "/api/version", nil))
	if versionResponse.Code != http.StatusOK || !strings.Contains(versionResponse.Body.String(), "currentVersion") {
		t.Fatalf("version status=%d body=%s", versionResponse.Code, versionResponse.Body.String())
	}
}
