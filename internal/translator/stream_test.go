package translator

import "testing"

func TestOpenAIChunkToClaudeSSE(t *testing.T) {
	state := &StreamState{}
	events := OpenAIChunkToClaudeSSE([]byte(`{"id":"chat-1","model":"x","choices":[{"delta":{"content":"hi"}}]}`), state)
	if len(events) != 3 {
		t.Fatalf("events = %d: %v", len(events), events)
	}
	finish := OpenAIChunkToClaudeSSE([]byte(`{"id":"chat-1","choices":[{"delta":{},"finish_reason":"stop"}]}`), state)
	if len(finish) != 3 {
		t.Fatalf("finish events = %d: %v", len(finish), finish)
	}
}
