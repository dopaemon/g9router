package translator

import "strings"

var codexResponseFields = map[string]bool{"model": true, "input": true, "instructions": true, "tools": true, "tool_choice": true, "stream": true, "store": true, "reasoning": true, "service_tier": true, "include": true, "prompt_cache_key": true, "client_metadata": true, "text": true}

func NormalizeCodexRequest(body map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range body {
		if codexResponseFields[key] {
			result[key] = value
		}
	}
	if instructions, ok := result["instructions"].(string); ok && instructions != "" {
		result["instructions"] = instructions
	}
	if items, ok := result["input"].([]any); ok {
		clean := make([]any, 0, len(items))
		for _, raw := range items {
			if id, ok := raw.(string); ok && (strings.HasPrefix(id, "rs_") || strings.HasPrefix(id, "fc_") || strings.HasPrefix(id, "resp_") || strings.HasPrefix(id, "msg_")) {
				continue
			}
			item, ok := raw.(map[string]any)
			if !ok {
				clean = append(clean, raw)
				continue
			}
			if kind, _ := item["type"].(string); kind == "item_reference" {
				continue
			}
			if id, ok := item["id"].(string); ok && (strings.HasPrefix(id, "rs_") || strings.HasPrefix(id, "fc_") || strings.HasPrefix(id, "resp_") || strings.HasPrefix(id, "msg_")) {
				delete(item, "id")
			}
			kind, _ := item["type"].(string)
			role, _ := item["role"].(string)
			if role == "system" && (kind == "" || kind == "message") {
				item["role"] = "developer"
			}
			clean = append(clean, item)
		}
		result["input"] = clean
	}
	if input, ok := result["input"].([]any); !ok || len(input) == 0 {
		result["input"] = []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "..."}}}}
	}
	if tools, ok := result["tools"].([]any); ok {
		validTools := normalizeCodexTools(tools)
		result["tools"] = validTools
		if choice, ok := result["tool_choice"].(map[string]any); ok && choice["type"] == "function" {
			name, _ := choice["name"].(string)
			if name == "" {
				delete(result, "tool_choice")
			} else {
				found := false
				for _, tool := range validTools {
					if tool.(map[string]any)["name"] == name {
						found = true
						break
					}
				}
				if !found {
					delete(result, "tool_choice")
				}
			}
		}
	}
	result["stream"] = true
	result["store"] = false
	return result
}

func normalizeCodexTools(tools []any) []any {
	result := make([]any, 0, len(tools))
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := tool["type"].(string)
		if kind != "function" {
			if kind == "namespace" || kind == "custom" || kind == "web_search" || kind == "web_search_preview" || kind == "file_search" || kind == "computer" || kind == "computer_use_preview" || kind == "code_interpreter" || kind == "mcp" || kind == "local_shell" || kind == "tool_search" {
				result = append(result, tool)
			}
			continue
		}
		fn, _ := tool["function"].(map[string]any)
		name, _ := tool["name"].(string)
		if name == "" {
			name, _ = fn["name"].(string)
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if len(name) > 128 {
			name = name[:128]
		}
		description, _ := tool["description"].(string)
		if description == "" {
			description, _ = fn["description"].(string)
		}
		parameters := tool["parameters"]
		if parameters == nil {
			parameters = fn["parameters"]
		}
		if parameters == nil {
			parameters = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		result = append(result, map[string]any{"type": "function", "name": name, "description": description, "parameters": parameters, "strict": tool["strict"]})
	}
	return result
}
