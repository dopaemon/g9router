package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyVertexStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.String(), ":streamGenerateContent") {
			t.Errorf("unexpected upstream path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"responseId":"r1","candidates":[{"content":{"parts":[{"text":"vertex"}]}}]}` + "\n"))
	}))
	defer upstream.Close()

	app := &Server{client: upstream.Client()}
	recorder := httptest.NewRecorder()
	incoming := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ok := app.proxyVertex(recorder, incoming, upstream.URL, "gemini-test", map[string]any{
		"model": "gemini-test", "stream": true, "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}, "key", false, nil)
	if !ok || recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"content":"vertex"`) {
		t.Fatalf("ok=%v status=%d body=%q", ok, recorder.Code, recorder.Body.String())
	}
}
