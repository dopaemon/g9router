package translator

import "testing"

func TestNormalizeCodexRequest(t *testing.T) {
	out := NormalizeCodexRequest(map[string]any{
		"model": "gpt-5.4", "input": []any{map[string]any{"type": "message", "role": "system", "id": "msg_old", "content": "hi"}, "resp_old"},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "Read", "parameters": map[string]any{"type": "object"}}}}, "temperature": 0.2,
	})
	if out["temperature"] != nil {
		t.Fatal(out)
	}
	items := out["input"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["role"] != "developer" {
		t.Fatal(out)
	}
	tools := out["tools"].([]any)
	if tools[0].(map[string]any)["name"] != "Read" {
		t.Fatal(out)
	}
	if out["stream"] != true || out["store"] != false || out["input"] == nil {
		t.Fatal(out)
	}
}

func TestNormalizeCodexRequestPreservesNonMessageSystemItems(t *testing.T) {
	out := NormalizeCodexRequest(map[string]any{
		"input": []any{map[string]any{"type": "reasoning", "role": "system", "content": "keep"}},
		"tools": []any{
			map[string]any{"type": "namespace"},
			map[string]any{"type": "local_shell"},
			map[string]any{"type": "tool_search"},
		},
	})
	item := out["input"].([]any)[0].(map[string]any)
	if item["role"] != "system" {
		t.Fatalf("item role = %#v", item["role"])
	}
	if len(out["tools"].([]any)) != 3 {
		t.Fatalf("tools = %#v", out["tools"])
	}
}
