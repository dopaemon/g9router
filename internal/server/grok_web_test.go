package server

import "testing"

func TestGrokMessageContent(t *testing.T) {
	content := grokMessageContent([]any{
		map[string]any{"type": "image_url", "image_url": "ignored"},
		map[string]any{"type": "text", "text": "first"},
		map[string]any{"type": "text", "text": "second"},
	})
	if content != "first second" {
		t.Fatalf("content = %q", content)
	}
}

func TestGrokModelCatalog(t *testing.T) {
	config, ok := grokModels["grok-4.1-mini"]
	if !ok || config.model != "grok-4-1-thinking-1129" || config.mode != "MODEL_MODE_GROK_4_1_MINI_THINKING" || !config.thinking {
		t.Fatalf("unexpected model config: %+v", config)
	}
}
