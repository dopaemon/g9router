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

func TestAPIHealthContract(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "*" || response.Body.String() != "{\"ok\":true}\n" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	options := httptest.NewRecorder()
	app.Handler().ServeHTTP(options, httptest.NewRequest(http.MethodOptions, "/api/health", nil))
	if options.Code != http.StatusNoContent {
		t.Fatalf("options status=%d", options.Code)
	}
}
