package translator

import (
	"encoding/json"
	"fmt"
)

type StreamState struct {
	Started     bool
	TextStarted bool
	TextClosed  bool
	ToolStarted map[int]bool
	NextIndex   int
}

func OpenAIChunkToClaudeSSE(raw []byte, state *StreamState) []string {
	var chunk map[string]any
	if json.Unmarshal(raw, &chunk) != nil {
		return nil
	}
	result := []string{}
	if !state.Started {
		state.Started = true
		result = append(result, event("message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": chunk["id"], "type": "message", "role": "assistant", "model": chunk["model"], "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}}))
	}
	choices := array(chunk["choices"])
	if len(choices) == 0 {
		return result
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	if content, ok := delta["content"].(string); ok && content != "" {
		if !state.TextStarted {
			state.TextStarted = true
			state.TextClosed = false
			result = append(result, event("content_block_start", map[string]any{"type": "content_block_start", "index": state.NextIndex, "content_block": map[string]any{"type": "text", "text": ""}}))
			state.NextIndex++
		}
		result = append(result, event("content_block_delta", map[string]any{"type": "content_block_delta", "index": state.NextIndex - 1, "delta": map[string]any{"type": "text_delta", "text": content}}))
	}
	if toolCalls := array(delta["tool_calls"]); len(toolCalls) > 0 {
		for _, rawCall := range toolCalls {
			call, _ := rawCall.(map[string]any)
			index := int(number(call["index"]))
			if !state.ToolStarted[index] {
				if state.TextStarted && !state.TextClosed {
					result = append(result, event("content_block_stop", map[string]any{"type": "content_block_stop", "index": state.NextIndex - 1}))
					state.TextClosed = true
				}
				state.ToolStarted[index] = true
				result = append(result, event("content_block_start", map[string]any{"type": "content_block_start", "index": state.NextIndex, "content_block": map[string]any{"type": "tool_use", "id": call["id"], "name": nestedString(call, "function", "name"), "input": map[string]any{}}}))
				state.NextIndex++
			}
			if args := nestedString(call, "function", "arguments"); args != "" {
				result = append(result, event("content_block_delta", map[string]any{"type": "content_block_delta", "index": state.NextIndex - 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": args}}))
			}
		}
	}
	if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
		if state.TextStarted && !state.TextClosed {
			result = append(result, event("content_block_stop", map[string]any{"type": "content_block_stop", "index": state.NextIndex - 1}))
			state.TextClosed = true
		}
		result = append(result, event("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": map[string]string{"stop": "end_turn", "tool_calls": "tool_use"}[reason], "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 0}}))
		result = append(result, event("message_stop", map[string]any{"type": "message_stop"}))
	}
	return result
}

func event(name string, value any) string {
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", name, encoded)
}
func nestedString(value map[string]any, keys ...string) string {
	current := any(value)
	for _, key := range keys {
		object, _ := current.(map[string]any)
		current = object[key]
	}
	text, _ := current.(string)
	return text
}
func number(value any) float64 { number, _ := value.(float64); return number }
