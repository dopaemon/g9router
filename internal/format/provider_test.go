package format

import "testing"

func TestDetectProviderFormats(t *testing.T) {
	if Detect(map[string]any{"input": "hi"}) != Responses {
		t.Fatal("responses")
	}
	if Detect(map[string]any{"system": "x", "messages": []any{}}) != Claude {
		t.Fatal("claude")
	}
	if Detect(map[string]any{"contents": []any{}}) != Gemini {
		t.Fatal("gemini")
	}
	if Target("anthropic-compatible-x") != Claude {
		t.Fatal("target")
	}
}
