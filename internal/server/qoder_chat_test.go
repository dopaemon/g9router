package server

import (
	"strings"
	"testing"
)

func TestQoderEncodeBody(t *testing.T) {
	got := string(qoderEncodeBody([]byte("abc")))
	if got != "MSK#" {
		t.Fatalf("encoded body = %q, want %q", got, "MSK#")
	}
}

func TestQoderMessagesHoistSystem(t *testing.T) {
	messages, system := qoderMessages([]any{
		map[string]any{"role": "system", "content": "first"},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hello"}}},
	})
	if system != "first" || len(messages) != 1 || messages[0]["content"] != "hello" {
		t.Fatalf("messages = %#v, system = %q", messages, system)
	}
}

func TestQoderUnwrapSSE(t *testing.T) {
	body := strings.NewReader("data: {\"statusCodeValue\":200,\"body\":\"{\\\"choices\\\":[{\\\"delta\\\":{\\\"content\\\":\\\"hi\\\"}}]}\"}\n\ndata: [DONE]\n")
	got := string(qoderUnwrapSSE(body, "qoder/qmodel"))
	if !strings.Contains(got, `"content":"hi"`) || !strings.HasSuffix(got, "data: [DONE]\n\n") {
		t.Fatalf("unwrapped SSE = %q", got)
	}
}
