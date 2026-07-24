package translator

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

const claudeSystemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."

func OpenAIToClaudeRequest(model string, body map[string]any, stream bool) map[string]any {
	result := map[string]any{"model": model, "max_tokens": maxTokens(body), "stream": stream, "messages": []any{}}
	if value, ok := body["temperature"]; ok {
		result["temperature"] = value
	}
	systemParts := []string{}
	messages := []any{}
	var role string
	parts := []any{}
	flush := func() {
		if role != "" && len(parts) > 0 {
			messages = append(messages, map[string]any{"role": role, "content": parts})
			role, parts = "", nil
		}
	}
	for _, raw := range array(body["messages"]) {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		messageRole, _ := message["role"].(string)
		if messageRole == "system" {
			systemParts = append(systemParts, textValue(message["content"]))
			continue
		}
		newRole := "assistant"
		if messageRole == "user" || messageRole == "tool" {
			newRole = "user"
		}
		blocks := openAIContentBlocks(message)
		toolResults := []any{}
		other := []any{}
		for _, block := range blocks {
			if block.(map[string]any)["type"] == "tool_result" {
				toolResults = append(toolResults, block)
			} else {
				other = append(other, block)
			}
		}
		if len(toolResults) > 0 {
			flush()
			messages = append(messages, map[string]any{"role": "user", "content": toolResults})
		}
		if len(other) == 0 {
			continue
		}
		if role != newRole {
			flush()
			role = newRole
		}
		parts = append(parts, other...)
		for _, block := range other {
			if block.(map[string]any)["type"] == "tool_use" {
				flush()
				break
			}
		}
	}
	flush()
	if len(messages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			message := messages[i].(map[string]any)
			if message["role"] == "assistant" {
				content := message["content"].([]any)
				for j := len(content) - 1; j >= 0; j-- {
					block := content[j].(map[string]any)
					if block["type"] != "thinking" {
						block["cache_control"] = map[string]any{"type": "ephemeral"}
						break
					}
				}
				break
			}
		}
	}
	result["messages"] = messages
	if format, ok := body["response_format"].(map[string]any); ok {
		switch format["type"] {
		case "json_object":
			systemParts = append(systemParts, "You must respond with valid JSON. Respond ONLY with a JSON object, no other text.")
		case "json_schema":
			if schema := nestedMap(format, "json_schema", "schema"); schema != nil {
				encoded, _ := json.MarshalIndent(schema, "", "  ")
				systemParts = append(systemParts, "You must respond with valid JSON that strictly follows this JSON schema:\n```json\n"+string(encoded)+"\n```\nRespond ONLY with the JSON object, no other text.")
			}
		}
	}
	result["system"] = []any{map[string]any{"type": "text", "text": claudeSystemPrompt}}
	if len(systemParts) > 0 {
		result["system"] = append(result["system"].([]any), map[string]any{"type": "text", "text": strings.Join(systemParts, "\n"), "cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}})
	}
	if tools := openAITools(body["tools"]); len(tools) > 0 {
		result["tools"] = tools
		tools[len(tools)-1].(map[string]any)["cache_control"] = map[string]any{"type": "ephemeral", "ttl": "1h"}
	}
	if choice, ok := body["tool_choice"]; ok {
		result["tool_choice"] = openAIToolChoice(choice)
	}
	return result
}

func maxTokens(body map[string]any) any {
	if value, ok := body["max_tokens"]; ok {
		return value
	}
	return 4096
}

func openAIContentBlocks(message map[string]any) []any {
	if message["role"] == "tool" {
		return []any{map[string]any{"type": "tool_result", "tool_use_id": message["tool_call_id"], "content": message["content"]}}
	}
	blocks := []any{}
	content := message["content"]
	if text, ok := content.(string); ok {
		if text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
	} else {
		for _, raw := range array(content) {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := part["type"].(string)
			switch kind {
			case "text":
				if text, _ := part["text"].(string); text != "" {
					blocks = append(blocks, map[string]any{"type": "text", "text": text})
				}
			case "image_url":
				if image := imageBlock(part); image != nil {
					blocks = append(blocks, image)
				}
			case "image":
				if part["source"] != nil {
					blocks = append(blocks, map[string]any{"type": "image", "source": part["source"]})
				}
			case "file":
				if file, ok := part["file"].(map[string]any); ok {
					if document := documentBlock(file["file_data"]); document != nil {
						blocks = append(blocks, document)
					}
				}
			}
		}
	}
	if message["role"] == "assistant" {
		for _, raw := range array(message["tool_calls"]) {
			call, _ := raw.(map[string]any)
			function, _ := call["function"].(map[string]any)
			input := map[string]any{}
			if json.Unmarshal([]byte(stringValue(function["arguments"])), &input) != nil {
				input = map[string]any{}
			}
			blocks = append(blocks, map[string]any{"type": "tool_use", "id": call["id"], "name": function["name"], "input": input})
		}
	}
	return blocks
}

func imageBlock(part map[string]any) map[string]any {
	image, _ := part["image_url"].(map[string]any)
	url, _ := image["url"].(string)
	if strings.HasPrefix(url, "data:") {
		mime, data, ok := dataURI(url)
		if ok {
			return map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": mime, "data": data}}
		}
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": url}}
	}
	return nil
}
func documentBlock(value any) map[string]any {
	url, _ := value.(string)
	mime, data, ok := dataURI(url)
	if ok && mime == "application/pdf" {
		return map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": mime, "data": data}}
	}
	return nil
}
func dataURI(value string) (string, string, bool) {
	if !strings.HasPrefix(value, "data:") {
		return "", "", false
	}
	pieces := strings.SplitN(value[5:], ",", 2)
	if len(pieces) != 2 || !strings.HasSuffix(pieces[0], ";base64") {
		return "", "", false
	}
	data, err := base64.StdEncoding.DecodeString(pieces[1])
	if err != nil {
		return "", "", false
	}
	return strings.TrimSuffix(pieces[0], ";base64"), base64.StdEncoding.EncodeToString(data), true
}
func openAITools(value any) []any {
	result := []any{}
	for _, raw := range array(value) {
		tool, _ := raw.(map[string]any)
		if tool["type"] != nil && tool["type"] != "function" {
			result = append(result, tool)
			continue
		}
		data, _ := tool["function"].(map[string]any)
		if data == nil {
			data = tool
		}
		name, _ := data["name"].(string)
		if name == "" {
			continue
		}
		schema := data["parameters"]
		if schema == nil {
			schema = data["input_schema"]
		}
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}}
		}
		result = append(result, map[string]any{"name": name, "description": data["description"], "input_schema": schema})
	}
	return result
}
func openAIToolChoice(value any) any {
	if choice, ok := value.(string); ok {
		if choice == "required" {
			return map[string]any{"type": "any"}
		}
		return map[string]any{"type": "auto"}
	}
	choice, _ := value.(map[string]any)
	if function, _ := choice["function"].(map[string]any); function != nil && function["name"] != nil {
		return map[string]any{"type": "tool", "name": function["name"]}
	}
	if kind, _ := choice["type"].(string); kind == "auto" || kind == "any" || kind == "tool" || kind == "none" {
		return choice
	}
	return map[string]any{"type": "auto"}
}
func nestedMap(value map[string]any, keys ...string) map[string]any {
	current := value
	for _, key := range keys {
		next, _ := current[key].(map[string]any)
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}
