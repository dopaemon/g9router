package translator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeChunkToOpenAISSETextAndFinish(t *testing.T) {
	state := &ClaudeStreamState{}
	start := ClaudeChunkToOpenAISSE([]byte(`{"type":"message_start","message":{"id":"msg-1","model":"claude"}}`), state)
	if len(start) != 1 || !strings.Contains(start[0], `"role":"assistant"`) {
		t.Fatal(start)
	}
	text := ClaudeChunkToOpenAISSE([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`), state)
	if len(text) != 1 || !strings.Contains(text[0], `"content":"hello"`) {
		t.Fatal(text)
	}
	finish := ClaudeChunkToOpenAISSE([]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`), state)
	if len(finish) != 1 || !strings.Contains(finish[0], `"finish_reason":"stop"`) {
		t.Fatal(finish)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(finish[0]), "data:"))), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["usage"] == nil {
		t.Fatal(payload)
	}
}

func TestClaudeChunkToOpenAISSEToolCall(t *testing.T) {
	state := &ClaudeStreamState{}
	ClaudeChunkToOpenAISSE([]byte(`{"type":"message_start","message":{"id":"msg-1","model":"claude"}}`), state)
	start := ClaudeChunkToOpenAISSE([]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call-1","name":"Read"}}`), state)
	if len(start) != 1 || !strings.Contains(start[0], `"name":"Read"`) {
		t.Fatal(start)
	}
	args := ClaudeChunkToOpenAISSE([]byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\":\"/tmp/a\"}"}}`), state)
	if len(args) != 1 || !strings.Contains(args[0], `file_path`) {
		t.Fatal(args)
	}
}
