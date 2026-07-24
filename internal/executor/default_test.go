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
