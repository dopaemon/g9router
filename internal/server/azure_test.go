package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"g9router/internal/providers"
)

func TestAzureDeploymentProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openai/deployments/chat-prod/chat/completions" || r.URL.Query().Get("api-version") != "2024-10-01-preview" {
			t.Fatalf("unexpected Azure endpoint: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("api-key") != "secret" {
			t.Fatal("missing Azure api-key")
		}
		fmt.Fprint(w, `{"id":"chatcmpl-azure"}`)
	}))
	defer upstream.Close()
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: os.TempDir() + "/azure-oauth.json"})
	if err := app.store.Upsert(providers.Provider{ID: "azure", APIKey: "secret", Enabled: true, ProviderSpecificData: map[string]any{"azureEndpoint": upstream.URL, "deployment": "chat-prod", "apiVersion": "2024-10-01-preview"}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"azure/chat-prod","messages":[]}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "chatcmpl-azure") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
