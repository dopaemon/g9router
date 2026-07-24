package executor

import "testing"

func TestApplyJSONSchemaFallback(t *testing.T) {
	out := ApplyJSONSchemaFallback(map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}, "response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"schema": map[string]any{"type": "object"}}}})
	if out["response_format"].(map[string]any)["type"] != "json_object" {
		t.Fatal(out)
	}
	if len(out["messages"].([]any)) != 2 {
		t.Fatal(out)
	}
}

func TestApplyJSONSchemaFallbackAppendsArrayContent(t *testing.T) {
	out := ApplyJSONSchemaFallback(map[string]any{
		"messages":        []any{map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": "rules"}}}},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"schema": map[string]any{"type": "object"}}},
	})
	content := out["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content = %#v", content)
	}
}
