package translator

import "encoding/json"

func ResponsesToChat(body map[string]any) map[string]any {
	if body["input"] == nil {
		return body
	}
	result := map[string]any{}
	for key, value := range body {
		result[key] = value
	}
	messages := []any{}
	if instructions, ok := body["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	var assistant map[string]any
	var toolResults []any
	items := array(body["input"])
	if text, ok := body["input"].(string); ok {
		items = []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": text}}}}
	}
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		kind, _ := item["type"].(string)
		if kind == "" {
			kind = "message"
		}
		switch kind {
		case "message":
			if assistant != nil {
				messages = append(messages, assistant)
				assistant = nil
			}
			content := item["content"]
			if parts := array(content); len(parts) > 0 {
				converted := []any{}
				for _, partRaw := range parts {
					part, _ := partRaw.(map[string]any)
					partType, _ := part["type"].(string)
					if partType == "input_text" || partType == "output_text" {
						converted = append(converted, map[string]any{"type": "text", "text": part["text"]})
					} else if partType == "input_image" {
						converted = append(converted, map[string]any{"type": "image_url", "image_url": map[string]any{"url": part["image_url"]}})
					}
				}
				content = converted
			}
			messages = append(messages, map[string]any{"role": item["role"], "content": content})
		case "function_call":
			if assistant == nil {
				assistant = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{}}
			}
			if name, _ := item["name"].(string); name != "" {
				calls := assistant["tool_calls"].([]any)
				calls = append(calls, map[string]any{"id": item["call_id"], "type": "function", "function": map[string]any{"name": name, "arguments": item["arguments"]}})
				assistant["tool_calls"] = calls
			}
		case "function_call_output":
			if assistant != nil {
				messages = append(messages, assistant)
				assistant = nil
			}
			output := item["output"]
			if _, ok := output.(string); !ok {
				encoded, _ := json.Marshal(output)
				output = string(encoded)
			}
			toolResults = append(toolResults, map[string]any{"role": "tool", "tool_call_id": item["call_id"], "content": output})
		}
	}
	if assistant != nil {
		messages = append(messages, assistant)
	}
	messages = append(messages, toolResults...)
	result["messages"] = messages
	delete(result, "input")
	delete(result, "instructions")
	delete(result, "store")
	delete(result, "reasoning")
	delete(result, "include")
	return result
}
