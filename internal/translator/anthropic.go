package translator

import (
	"encoding/json"
	"fmt"
)

func ClaudeToOpenAI(model string, body map[string]any, stream bool) map[string]any {
	out := map[string]any{"model": model, "messages": []any{}, "stream": stream}
	if value, ok := body["max_tokens"]; ok {
		out["max_tokens"] = value
	}
	if value, ok := body["temperature"]; ok {
		out["temperature"] = value
	}
	if system := textValue(body["system"]); system != "" {
		out["messages"] = append(out["messages"].([]any), map[string]any{"role": "system", "content": system})
	}
	for _, raw := range array(body["messages"]) {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		converted := claudeMessage(message)
		out["messages"] = append(out["messages"].([]any), converted...)
	}
	if tools := array(body["tools"]); len(tools) > 0 {
		out["tools"] = claudeTools(tools)
	}
	if choice, ok := body["tool_choice"]; ok {
		out["tool_choice"] = toolChoice(choice)
	}
	return out
}

func OpenAIToClaudeResponse(body map[string]any) map[string]any {
	choiceList := array(body["choices"])
	message := map[string]any{}
	if len(choiceList) > 0 {
		if choice, ok := choiceList[0].(map[string]any); ok {
			message, _ = choice["message"].(map[string]any)
		}
	}
	content := []any{}
	if text, ok := message["content"].(string); ok && text != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	for _, raw := range array(message["tool_calls"]) {
		call, _ := raw.(map[string]any)
		function, _ := call["function"].(map[string]any)
		var input any = map[string]any{}
		_ = json.Unmarshal([]byte(stringValue(function["arguments"])), &input)
		content = append(content, map[string]any{"type": "tool_use", "id": call["id"], "name": function["name"], "input": input})
	}
	stopReason := "end_turn"
	if len(choiceList) > 0 {
		if choice, ok := choiceList[0].(map[string]any); ok {
			if reason, ok := choice["finish_reason"].(string); ok && reason == "tool_calls" {
				stopReason = "tool_use"
			}
		}
	}
	usage := map[string]any{"input_tokens": 0, "output_tokens": 0}
	if source, ok := body["usage"].(map[string]any); ok {
		usage["input_tokens"] = source["prompt_tokens"]
		usage["output_tokens"] = source["completion_tokens"]
	}
	return map[string]any{"id": body["id"], "type": "message", "role": "assistant", "model": body["model"], "content": content, "stop_reason": stopReason, "stop_sequence": nil, "usage": usage}
}

func claudeMessage(message map[string]any) []any {
	role, _ := message["role"].(string)
	if role == "tool" {
		role = "user"
	}
	content, ok := message["content"].(string)
	if ok {
		return []any{map[string]any{"role": role, "content": content}}
	}
	parts := array(message["content"])
	textParts := []any{}
	toolCalls := []any{}
	toolResults := []any{}
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := part["type"].(string)
		switch kind {
		case "text":
			text, _ := part["text"].(string)
			textParts = append(textParts, map[string]any{"type": "text", "text": text})
		case "image":
			source, _ := part["source"].(map[string]any)
			if source["type"] == "base64" {
				media, _ := source["media_type"].(string)
				data, _ := source["data"].(string)
				textParts = append(textParts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", media, data)}})
			}
		case "tool_use":
			input := part["input"]
			encoded, _ := json.Marshal(input)
			toolCalls = append(toolCalls, map[string]any{"id": part["id"], "type": "function", "function": map[string]any{"name": part["name"], "arguments": string(encoded)}})
		case "tool_result":
			toolResults = append(toolResults, map[string]any{"role": "tool", "tool_call_id": part["tool_use_id"], "content": textValue(part["content"])})
		}
	}
	result := []any{}
	if len(toolResults) > 0 {
		result = append(result, toolResults...)
	}
	if len(toolCalls) > 0 {
		assistant := map[string]any{"role": "assistant", "tool_calls": toolCalls}
		if len(textParts) > 0 {
			assistant["content"] = collapseText(textParts)
		}
		result = append(result, assistant)
		return result
	}
	if len(textParts) > 0 {
		result = append(result, map[string]any{"role": role, "content": collapseText(textParts)})
	}
	return result
}

func claudeTools(tools []any) []any {
	result := make([]any, 0, len(tools))
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		result = append(result, map[string]any{"type": "function", "function": map[string]any{"name": tool["name"], "description": tool["description"], "parameters": tool["input_schema"]}})
	}
	return result
}
func toolChoice(choice any) any {
	value, ok := choice.(map[string]any)
	if !ok {
		return choice
	}
	kind, _ := value["type"].(string)
	if kind == "auto" || kind == "none" || kind == "any" {
		if kind == "any" {
			return "required"
		}
		return kind
	}
	if name, ok := value["name"]; ok {
		return map[string]any{"type": "function", "function": map[string]any{"name": name}}
	}
	return choice
}
func textValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	parts := array(value)
	result := ""
	for _, raw := range parts {
		part, _ := raw.(map[string]any)
		if text, ok := part["text"].(string); ok {
			if result != "" {
				result += "\n"
			}
			result += text
		}
	}
	return result
}
func stringValue(value any) string { text, _ := value.(string); return text }
func collapseText(parts []any) any {
	if len(parts) == 1 {
		if part, ok := parts[0].(map[string]any); ok && part["type"] == "text" {
			return part["text"]
		}
	}
	return parts
}
func array(value any) []any { result, _ := value.([]any); return result }

func ClaudeResponseToOpenAI(body map[string]any) map[string]any {
	content := []any{}
	toolCalls := []any{}
	for _, raw := range array(body["content"]) {
		block, _ := raw.(map[string]any)
		kind, _ := block["type"].(string)
		switch kind {
		case "text":
			if text, _ := block["text"].(string); text != "" {
				content = append(content, map[string]any{"type": "text", "text": text})
			}
		case "tool_use":
			encoded, _ := json.Marshal(block["input"])
			toolCalls = append(toolCalls, map[string]any{
				"id": block["id"], "type": "function",
				"function": map[string]any{"name": block["name"], "arguments": string(encoded)},
			})
		}
	}
	message := map[string]any{"role": "assistant"}
	if len(content) == 1 {
		message["content"] = content[0].(map[string]any)["text"]
	} else if len(content) > 0 {
		message["content"] = content
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	finishReason := "stop"
	if reason, _ := body["stop_reason"].(string); reason == "tool_use" {
		finishReason = "tool_calls"
	} else if reason == "max_tokens" {
		finishReason = "length"
	}
	choice := map[string]any{"index": 0, "message": message, "finish_reason": finishReason}
	result := map[string]any{"id": body["id"], "object": "chat.completion", "model": body["model"], "choices": []any{choice}}
	if usage, ok := body["usage"].(map[string]any); ok {
		prompt := usageNumber(usage["input_tokens"])
		completion := usageNumber(usage["output_tokens"])
		result["usage"] = map[string]any{"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": prompt + completion}
	}
	return result
}

func usageNumber(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case float64:
		return int(number)
	default:
		return 0
	}
}
