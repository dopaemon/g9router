package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"g9router/internal/providers"
)

func TestChatCompletionProxiesRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer client-key" {
			t.Errorf("authorization was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"test"}`)
	}))
	defer upstream.Close()
	server := httptest.NewServer(New(Options{Upstream: upstream.URL + "/v1", ProviderPath: os.TempDir() + "/g9router-test-1.json"}).Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[]}`))
	request.Header.Set("Authorization", "Bearer client-key")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	t.Logf("status=%d content-type=%s", response.StatusCode, response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestModelsUsesGET(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer upstream.Close()
	server := httptest.NewServer(New(Options{Upstream: upstream.URL + "/v1", ProviderPath: os.TempDir() + "/g9router-test-2.json"}).Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestClaudeMessagesTranslationRoundTrip(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode upstream request: %v", err)
			return
		}
		messages, ok := request["messages"].([]any)
		if !ok || len(messages) == 0 || messages[0].(map[string]any)["role"] != "user" {
			t.Errorf("unexpected translated request: %v", request)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat-1","model":"x","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hello"}}]}`)
	}))
	defer upstream.Close()
	app := New(Options{Upstream: upstream.URL + "/v1", ProviderPath: os.TempDir() + "/g9router-test-3.json"})
	if err := app.store.Upsert(providers.Provider{ID: "openai", BaseURL: upstream.URL + "/v1", APIType: "openai", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/messages", strings.NewReader(`{"model":"openai/gpt","messages":[{"role":"user","content":"hi"}]}`))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d body=%q", response.StatusCode, raw)
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result["type"] != "message" || result["role"] != "assistant" {
		t.Fatal(result)
	}
}
