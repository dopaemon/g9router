package chatcore

import "testing"

func TestStripUnsupportedUsesTurnAwarePlaceholder(t *testing.T) {
	body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "x"}}}}}}
	if !StripUnsupported(body, "openai", Capabilities{}) {
		t.Fatal("not stripped")
	}
	content := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["text"] != "[image omitted: model has no vision support]" {
		t.Fatal(content)
	}
}
