package translator

import "testing"

func TestOpenAIToKiroBuildsConversationState(t *testing.T) {
	out := OpenAIToKiro("kiro", map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}}, true)
	state := out["conversationState"].(map[string]any)
	if state["currentMessage"] == nil || out["agentMode"] != "vibe" {
		t.Fatal(out)
	}
}

func TestOpenAIToKiroPreservesSessionIdentity(t *testing.T) {
	out := OpenAIToKiro("kiro", map[string]any{"conversation_id": "conv-1", "agent_continuation_id": "cont-2", "messages": []any{map[string]any{"role": "user", "content": "hello"}}}, true)
	state := out["conversationState"].(map[string]any)
	if state["conversationId"] != "conv-1" || state["agentContinuationId"] != "cont-2" {
		t.Fatal(state)
	}
}
