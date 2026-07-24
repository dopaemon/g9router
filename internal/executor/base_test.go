package executor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExecuteRetriesRateLimit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()
	result, err := Execute(t.Context(), nil, Config{BaseURLs: []string{server.URL}, RetryAttempts: 2, RetryDelay: time.Millisecond}, "/v1", nil, false)
	if err != nil || string(result.Body) != "ok" || calls != 2 {
		t.Fatal(result, err, calls)
	}
}

func TestExecuteRetriesCodexTransientSSE(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `event: error\ndata: {"code":"server_is_overloaded"}`)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()
	result, err := Execute(t.Context(), server.Client(), Config{Provider: "codex", BaseURLs: []string{server.URL}, RetryAttempts: 2, RetryDelay: time.Millisecond}, "/responses", nil, true)
	if err != nil || string(result.Body) != "ok" || calls != 2 {
		t.Fatal(result, err, calls)
	}
}
