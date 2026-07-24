package translator

import "testing"

func TestChatToResponsesResponse(t *testing.T) {
	out := ChatToResponsesResponse(map[string]any{"id": "c", "model": "x", "choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "hi"}}}})
	if out["status"] != "completed" || len(out["output"].([]any)) != 1 {
		t.Fatal(out)
	}
}
