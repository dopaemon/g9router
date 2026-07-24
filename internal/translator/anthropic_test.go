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
