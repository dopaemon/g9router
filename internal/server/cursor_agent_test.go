package server

import "testing"

func TestCursorAgentEligible(t *testing.T) {
	if !cursorAgentEligible(map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}}) {
		t.Fatal("text request rejected")
	}
	if cursorAgentEligible(map[string]any{"messages": []any{map[string]any{"role": "tool", "content": "result"}}}) {
		t.Fatal("tool request accepted")
	}
	if cursorAgentEligible(map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url"}}}}}) {
		t.Fatal("non-text content accepted")
	}
}
