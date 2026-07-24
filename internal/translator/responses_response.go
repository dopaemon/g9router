package translator

func ResponsesToChatResponse(body map[string]any) map[string]any {
	content := ""
	toolCalls := []any{}
	for _, raw := range array(body["output"]) {
		item, _ := raw.(map[string]any)
		switch item["type"] {
		case "message":
			for _, partRaw := range array(item["content"]) {
				part, _ := partRaw.(map[string]any)
				if text, ok := part["text"].(string); ok {
					content += text
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, map[string]any{"id": item["call_id"], "type": "function", "function": map[string]any{"name": item["name"], "arguments": item["arguments"]}})
		}
	}
	message := map[string]any{"role": "assistant", "content": content}
	finish := "stop"
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		finish = "tool_calls"
	}
	result := map[string]any{"id": body["id"], "object": "chat.completion", "model": body["model"], "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}}
	if usage, ok := body["usage"]; ok {
		result["usage"] = usage
	}
	return result
}
