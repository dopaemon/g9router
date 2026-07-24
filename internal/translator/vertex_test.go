package translator

import "testing"

func TestOpenAIToVertexStripsFunctionIDs(t *testing.T) {
	out := OpenAIToVertex("gemini", map[string]any{"messages": []any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call-1", "type": "function", "function": map[string]any{"name": "Read", "arguments": "{}"}}}}}})
	if out["contents"] == nil {
		t.Fatal(out)
	}
}
