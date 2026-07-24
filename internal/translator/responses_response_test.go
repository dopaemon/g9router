package translator

import "testing"

func TestResponsesToChatResponseToolCall(t *testing.T) {
	out := ResponsesToChatResponse(map[string]any{"id": "r", "model": "x", "output": []any{map[string]any{"type": "function_call", "call_id": "c", "name": "Read", "arguments": "{}"}}})
	choice := out["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" {
		t.Fatal(out)
	}
}
