package translator

import (
	"encoding/json"
	"fmt"
)

type ResponsesStreamState struct {
	Started   bool
	ToolIndex int
}

func ResponsesEventToChatSSE(raw []byte, state *ResponsesStreamState) []string {
	var event map[string]any
	if json.Unmarshal(raw, &event) != nil {
		return nil
	}
	kind, _ := event["type"].(string)
	out := []string{}
	if !state.Started {
		state.Started = true
		out = append(out, sse(map[string]any{"id": event["response_id"], "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}}}))
	}
	switch kind {
	case "response.output_text.delta":
		if delta, ok := event["delta"].(string); ok {
			out = append(out, sse(map[string]any{"id": event["response_id"], "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": delta}, "finish_reason": nil}}}))
		}
	case "response.function_call_arguments.delta":
		if delta, ok := event["delta"].(string); ok {
			out = append(out, sse(map[string]any{"id": event["response_id"], "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": state.ToolIndex, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": state.ToolIndex, "function": map[string]any{"arguments": delta}}}}, "finish_reason": nil}}}))
		}
	case "response.completed":
		out = append(out, sse(map[string]any{"id": event["response_id"], "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}}), "data: [DONE]\n\n")
	case "response.failed":
		out = append(out, sse(map[string]any{"id": event["response_id"], "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}}), "data: [DONE]\n\n")
	}
	return out
}
func sse(value any) string {
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("data: %s\n\n", encoded)
}
