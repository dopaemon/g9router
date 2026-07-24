package chatcore

import "testing"

func TestFixMissingToolResponses(t *testing.T) {
	messages := FixMissingToolResponses([]any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "call-1"}, map[string]any{"id": "call-2"}}}, map[string]any{"role": "tool", "tool_call_id": "call-1", "content": "ok"}})
	if len(messages) != 3 {
		t.Fatal(messages)
	}
	if messages[2].(map[string]any)["tool_call_id"] != "call-2" {
		t.Fatal(messages)
	}
}
