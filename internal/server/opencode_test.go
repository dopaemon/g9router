package server

import (
	"encoding/json"
	"testing"
)

func TestInjectOpenCodeReasoning(t *testing.T) {
	body := injectOpenCodeReasoning([]byte(`{"model":"deepseek-v4-pro","messages":[{"role":"assistant","content":"x"}]}`))
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	message := payload["messages"].([]any)[0].(map[string]any)
	if message["reasoning_content"] != " " {
		t.Fatalf("reasoning_content = %#v", message["reasoning_content"])
	}
}
