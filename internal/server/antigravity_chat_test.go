package server

import (
	"strings"
	"testing"
)

func TestAntigravitySSE(t *testing.T) {
	body := strings.NewReader(`data: {"responseId":"r1","candidates":[{"content":{"parts":[{"text":"hello"}]}}]}` + "\n")
	got := antigravitySSE(body, "gemini-3-flash")
	if !strings.Contains(got, `"content":"hello"`) || !strings.HasSuffix(got, "data: [DONE]\n\n") {
		t.Fatalf("SSE output = %q", got)
	}
}
