package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"g9router/internal/providers"
)

func TestProviderProxyBaseURLUsesTokenPlanRegion(t *testing.T) {
	provider := providers.Provider{ID: "xiaomi-tokenplan", BaseURL: "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions", ProviderSpecificData: map[string]any{"region": "ams"}}
	if got := providerProxyBaseURL(provider); got != "https://token-plan-ams.xiaomimimo.com/v1" {
		t.Fatalf("base URL = %q", got)
	}
}

func TestValidateAzureRequiresEndpoint(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"azure","apiKey":"secret","providerSpecificData":{}}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Missing Azure endpoint") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateCloudflareRequiresAccount(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"cloudflare-ai","apiKey":"secret","providerSpecificData":{}}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Missing Account ID") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
