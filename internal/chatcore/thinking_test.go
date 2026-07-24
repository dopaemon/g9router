package chatcore

import "testing"

func TestNormalizeThinking(t *testing.T) {
	out := NormalizeThinking(map[string]any{"messages": []any{map[string]any{"role": "user"}}}, "on")
	if out["thinking"] == nil {
		t.Fatal(out)
	}
	out = NormalizeThinking(map[string]any{"thinking": map[string]any{"type": "enabled"}, "messages": []any{map[string]any{"role": "assistant"}}}, "auto")
	if out["thinking"] != nil {
		t.Fatal(out)
	}
}
