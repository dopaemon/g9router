package translator

import "encoding/json"

func OpenAIToGemini(model string, body map[string]any) map[string]any {
	result := map[string]any{"contents": []any{}}
	contents := result["contents"].([]any)
	for _, raw := range array(body["messages"]) {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		if role == "assistant" {
			role = "model"
		}
		if role == "system" {
			continue
		}
		parts := []any{}
		if text, ok := message["content"].(string); ok {
			parts = append(parts, map[string]any{"text": text})
		} else {
			for _, part := range array(message["content"]) {
				item, _ := part.(map[string]any)
				if text, ok := item["text"].(string); ok {
					parts = append(parts, map[string]any{"text": text})
				}
			}
		}
		if len(parts) > 0 {
			contents = append(contents, map[string]any{"role": role, "parts": parts})
		}
	}
	result["contents"] = contents
	if system, ok := body["system"].(string); ok {
		result["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": system}}}
	}
	generation := map[string]any{}
	for _, key := range []string{"temperature", "top_p", "top_k", "max_tokens"} {
		if value, ok := body[key]; ok {
			target := key
			if key == "max_tokens" {
				target = "maxOutputTokens"
			}
			generation[target] = value
		}
	}
	if len(generation) > 0 {
		result["generationConfig"] = generation
	}
	if tools := array(body["tools"]); len(tools) > 0 {
		functions := []any{}
		for _, raw := range tools {
			tool, _ := raw.(map[string]any)
			function, _ := tool["function"].(map[string]any)
			parameters := function["parameters"]
			functions = append(functions, map[string]any{"name": function["name"], "description": function["description"], "parameters": parameters})
		}
		result["tools"] = []any{map[string]any{"functionDeclarations": functions}}
	}
	_ = model
	return result
}

func GeminiToOpenAI(model string, body map[string]any) map[string]any {
	content := ""
	toolCalls := []any{}
	for _, raw := range array(body["candidates"]) {
		candidate, _ := raw.(map[string]any)
		contentObj, _ := candidate["content"].(map[string]any)
		for _, partRaw := range array(contentObj["parts"]) {
			part, _ := partRaw.(map[string]any)
			if text, ok := part["text"].(string); ok {
				content += text
			}
			if call, ok := part["functionCall"].(map[string]any); ok {
				args, _ := json.Marshal(call["args"])
				toolCalls = append(toolCalls, map[string]any{"id": call["name"], "type": "function", "function": map[string]any{"name": call["name"], "arguments": string(args)}})
			}
		}
	}
	message := map[string]any{"role": "assistant", "content": content}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	return map[string]any{"id": body["responseId"], "model": model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": "stop"}}, "usage": body["usageMetadata"]}
}
