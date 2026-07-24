package executor

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfigForProviderUsesRegistry(t *testing.T) {
	config, ok := ConfigForProvider("anthropic", "key")
	if !ok || config.Format != "claude" || config.Headers["anthropic-version"] == "" {
		t.Fatal(config, ok)
	}
	if config.APIKey != "key" {
		t.Fatal(config.APIKey)
	}
}

func TestExecuteUsesGeminiAPIKeyHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Goog-Api-Key") != "secret" {
			t.Fatalf("header = %q", r.Header.Get("X-Goog-Api-Key"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	result, err := Execute(t.Context(), server.Client(), Config{BaseURLs: []string{server.URL}, Format: "gemini", APIKey: "secret"}, "/models", nil, false)
	if err != nil || result.Status != http.StatusOK {
		t.Fatal(result, err)
	}
}
