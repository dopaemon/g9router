package translator

import (
	"encoding/json"
	"fmt"
	"time"
)

type ClaudeStreamState struct {
	MessageID        string
	Model            string
	ToolCallIndex    int
	ToolCalls        map[int]*streamToolCall
	Usage            map[string]int
	FinishReasonSent bool
	InThinking       bool
	ThinkingIndex    int
	ServerToolIndex  int
}

type streamToolCall struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

func ClaudeChunkToOpenAISSE(raw []byte, state *ClaudeStreamState) []string {
	var chunk map[string]any
	if json.Unmarshal(raw, &chunk) != nil {
		return nil
	}
	if state.ToolCalls == nil {
		state.ToolCalls = map[int]*streamToolCall{}
	}
	if state.ServerToolIndex == 0 {
		state.ServerToolIndex = -1
	}
	results := []string{}
	eventType, _ := chunk["type"].(string)
	base := func(delta map[string]any, finish any) string {
		choice := map[string]any{"index": 0, "delta": delta, "finish_reason": finish}
		return claudeSSE(map[string]any{"id": "chatcmpl-" + state.MessageID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": state.Model, "choices": []any{choice}})
	}
	switch eventType {
	case "message_start":
		message, _ := chunk["message"].(map[string]any)
		state.MessageID, _ = message["id"].(string)
		state.Model, _ = message["model"].(string)
		state.Usage = usageInts(message["usage"])
		results = append(results, base(map[string]any{"role": "assistant"}, nil))
	case "content_block_start":
		index := intValue(chunk["index"])
		block, _ := chunk["content_block"].(map[string]any)
		kind, _ := block["type"].(string)
		if kind == "server_tool_use" {
			state.ServerToolIndex = index
			break
		}
		if index == state.ServerToolIndex {
			break
		}
		switch kind {
		case "thinking":
			state.InThinking = true
			state.ThinkingIndex = index
			results = append(results, base(map[string]any{"content": "<think>"}, nil))
		case "tool_use":
			name, _ := block["name"].(string)
			id, _ := block["id"].(string)
			call := &streamToolCall{Index: state.ToolCallIndex, ID: id, Name: name}
			state.ToolCalls[index] = call
			state.ToolCallIndex++
			results = append(results, base(map[string]any{"tool_calls": []any{map[string]any{"index": call.Index, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": ""}}}}, nil))
		}
	case "content_block_delta":
		index := intValue(chunk["index"])
		if index == state.ServerToolIndex {
			break
		}
		delta, _ := chunk["delta"].(map[string]any)
		kind, _ := delta["type"].(string)
		switch kind {
		case "text_delta":
			if text, _ := delta["text"].(string); text != "" {
				results = append(results, base(map[string]any{"content": text}, nil))
			}
		case "thinking_delta":
			if text, _ := delta["thinking"].(string); text != "" {
				results = append(results, base(map[string]any{"content": text}, nil))
			}
		case "input_json_delta":
			if partial, _ := delta["partial_json"].(string); partial != "" {
				if call := state.ToolCalls[index]; call != nil {
					call.Arguments += partial
					results = append(results, base(map[string]any{"tool_calls": []any{map[string]any{"index": call.Index, "id": call.ID, "function": map[string]any{"arguments": partial}}}}, nil))
				}
			}
		}
	case "content_block_stop":
		index := intValue(chunk["index"])
		if index == state.ServerToolIndex {
			state.ServerToolIndex = -1
			break
		}
		if state.InThinking && index == state.ThinkingIndex {
			results = append(results, base(map[string]any{"content": "</think>"}, nil))
			state.InThinking = false
		}
	case "message_delta":
		state.Usage = mergeUsage(state.Usage, usageInts(chunk["usage"]))
		delta, _ := chunk["delta"].(map[string]any)
		reason, _ := delta["stop_reason"].(string)
		if reason != "" && !state.FinishReasonSent {
			result := base(map[string]any{}, openAIFinish(reason))
			if len(state.Usage) > 0 {
				result = addUsage(result, state.Usage)
			}
			results = append(results, result)
			state.FinishReasonSent = true
		}
	case "message_stop":
		if !state.FinishReasonSent {
			reason := "stop"
			if len(state.ToolCalls) > 0 {
				reason = "tool_calls"
			}
			result := base(map[string]any{}, reason)
			if len(state.Usage) > 0 {
				result = addUsage(result, state.Usage)
			}
			results = append(results, result)
			state.FinishReasonSent = true
		}
	}
	return results
}

func claudeSSE(value any) string {
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("data: %s\n\n", encoded)
}
func intValue(value any) int { number, _ := value.(float64); return int(number) }
func usageInts(value any) map[string]int {
	result := map[string]int{}
	object, _ := value.(map[string]any)
	for source, target := range map[string]string{"input_tokens": "prompt_tokens", "output_tokens": "completion_tokens", "cache_read_input_tokens": "cache_read_input_tokens", "cache_creation_input_tokens": "cache_creation_input_tokens"} {
		if number, ok := object[source].(float64); ok {
			result[target] = int(number)
		}
	}
	if result["prompt_tokens"] > 0 || result["completion_tokens"] > 0 {
		result["total_tokens"] = result["prompt_tokens"] + result["completion_tokens"]
	}
	return result
}
func mergeUsage(current, next map[string]int) map[string]int {
	if current == nil {
		current = map[string]int{}
	}
	for key, value := range next {
		current[key] = value
	}
	if current["prompt_tokens"] > 0 || current["completion_tokens"] > 0 {
		current["total_tokens"] = current["prompt_tokens"] + current["completion_tokens"]
	}
	return current
}
func openAIFinish(reason string) string {
	if reason == "tool_use" {
		return "tool_calls"
	}
	if reason == "max_tokens" {
		return "length"
	}
	return "stop"
}
func addUsage(raw string, usage map[string]int) string {
	var value map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return raw
	}
	value["usage"] = usage
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("data: %s\n\n", encoded)
}
