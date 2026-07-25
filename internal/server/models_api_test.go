package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelsAPI(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"fullModel":"openai/gpt-5.4"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestModelInfoOptions(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodOptions, "/v1/models/info", nil))
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
}

func TestModelCatalogParity(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models/embedding", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "text-embedding-3-small") {
		t.Fatalf("embedding catalog status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models/info?id=openai/gpt-4o", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"endpoint":"/v1/chat/completions"`) {
		t.Fatalf("model info status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models/info?id=el/eleven_multilingual_v2", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"voicesUrl":"/v1/audio/voices?provider=elevenlabs"`) {
		t.Fatalf("tts model info status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models/info?id=perplexity-web/search", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"endpoint":"/v1/search"`) {
		t.Fatalf("search model info status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/models/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown kind status=%d", response.Code)
	}
}

func TestCLIToolGuidesAPI(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/cli-tools/guides", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"claude"`) || !strings.Contains(response.Body.String(), `"id":"opencode"`) {
		t.Fatalf("guide metadata status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestModelsEndpointOptions(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodOptions, "/v1/models", nil))
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
}
