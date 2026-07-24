package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardChatAlias(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"chat-1","choices":[]}`))
	}))
	defer upstream.Close()
	app := New(Options{Upstream: upstream.URL, ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	request := httptest.NewRequest(http.MethodPost, "/api/dashboard/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[]}`))
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "chat-1") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
