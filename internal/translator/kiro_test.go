package translator

import "testing"

func TestOpenAIToKiroBuildsConversationState(t *testing.T) {
	out := OpenAIToKiro("kiro", map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}}, true)
	state := out["conversationState"].(map[string]any)
	if state["currentMessage"] == nil || out["agentMode"] != "vibe" {
		t.Fatal(out)
	}
}
