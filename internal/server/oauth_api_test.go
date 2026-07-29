package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"g9router/internal/oauth"
)

func TestOAuthAPIHidesCredentialSecrets(t *testing.T) {
	app := New(Options{OAuthPath: t.TempDir() + "/oauth.json"})
	if err := app.oauth.Upsert(oauth.Credential{ID: "kiro", Provider: "kiro", AccessToken: "access", RefreshToken: "refresh", ProviderSpecificData: map[string]any{
		"clientId": "client", "clientSecret": "secret", "region": "us-east-1",
	}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/oauth", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || strings.Contains(body, "access") || strings.Contains(body, "refresh") || strings.Contains(body, "secret") || !strings.Contains(body, "us-east-1") {
		t.Fatalf("status=%d body=%s", response.Code, body)
	}
}
