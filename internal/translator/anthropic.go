package translator

import (
	"encoding/base64"
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
func collapseText(parts []any) any {
	if len(parts) == 1 {
		if part, ok := parts[0].(map[string]any); ok && part["type"] == "text" {
			return part["text"]
		}
	}
	return parts
}
func array(value any) []any { result, _ := value.([]any); return result }

var _ = base64.StdEncoding
