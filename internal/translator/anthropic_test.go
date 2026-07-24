package translator

import "testing"

func TestClaudeToOpenAIConvertsToolsAndImages(t *testing.T) {
	out := ClaudeToOpenAI("gpt-test", map[string]any{
		"system": "be concise", "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi"}, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "abc"}}}}},
		"tools": []any{map[string]any{"name": "Read", "input_schema": map[string]any{"type": "object"}}},
	}, true)
	if out["model"] != "gpt-test" || out["stream"] != true {
		t.Fatal(out)
	}
	if len(out["tools"].([]any)) != 1 {
		t.Fatal(out)
	}
	messages := out["messages"].([]any)
	if len(messages) != 2 {
		t.Fatal(messages)
	}
	content := messages[1].(map[string]any)["content"].([]any)
	if content[1].(map[string]any)["type"] != "image_url" {
		t.Fatal(content)
	}
}

func TestClaudeResponseToOpenAI(t *testing.T) {
	out := ClaudeResponseToOpenAI(map[string]any{
		"id": "msg-1", "model": "claude", "stop_reason": "tool_use",
		"content": []any{map[string]any{"type": "text", "text": "ready"}, map[string]any{"type": "tool_use", "id": "call-1", "name": "Read", "input": map[string]any{"path": "/tmp/a"}}},
		"usage":   map[string]any{"input_tokens": float64(4), "output_tokens": float64(2)},
	})
	choices := out["choices"].([]any)
	choice := choices[0].(map[string]any)
	if choice["finish_reason"] != "tool_calls" || choice["message"].(map[string]any)["tool_calls"] == nil {
		t.Fatal(out)
	}
}
