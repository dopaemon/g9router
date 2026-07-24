package translator

import "testing"

func TestOpenAIToClaudeResponseConvertsToolCall(t *testing.T) {
	arguments := `{"file_path":"a.go"}`
	call := map[string]any{"id": "call-1", "function": map[string]any{"name": "Read", "arguments": arguments}}
	message := map[string]any{"tool_calls": []any{call}}
	choice := map[string]any{"finish_reason": "tool_calls", "message": message}
	out := OpenAIToClaudeResponse(map[string]any{"id": "chat-1", "model": "x", "choices": []any{choice}})
	if out["stop_reason"] != "tool_use" {
		t.Fatal(out)
	}
	content := out["content"].([]any)
	if content[0].(map[string]any)["type"] != "tool_use" {
		t.Fatal(content)
	}
}
