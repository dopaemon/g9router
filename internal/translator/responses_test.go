package translator

import "testing"

func TestResponsesToChat(t *testing.T) {
	out := ResponsesToChat(map[string]any{"instructions": "sys", "input": []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hi"}}}, map[string]any{"type": "function_call", "call_id": "c", "name": "Read", "arguments": "{}"}, map[string]any{"type": "function_call_output", "call_id": "c", "output": "ok"}}})
	messages := out["messages"].([]any)
	if len(messages) != 4 {
		t.Fatal(messages)
	}
	if out["input"] != nil {
		t.Fatal(out)
	}
}
