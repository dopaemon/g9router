package executor

import (
	"encoding/json"
	"fmt"
)

func ApplyJSONSchemaFallback(body map[string]any) map[string]any {
	responseFormat, _ := body["response_format"].(map[string]any)
	if responseFormat["type"] != "json_schema" {
		return body
	}
	schema, _ := responseFormat["json_schema"].(map[string]any)
	if schema["schema"] == nil {
		return body
	}
	encoded, _ := json.MarshalIndent(schema["schema"], "", "  ")
	prompt := fmt.Sprintf("You must respond with valid JSON that strictly follows this JSON schema:\n```json\n%s\n```\nRespond ONLY with the JSON object, no other text.", encoded)
	result := map[string]any{}
	for key, value := range body {
		result[key] = value
	}
	messages, _ := body["messages"].([]any)
	copied := make([]any, len(messages))
	copy(copied, messages)
	added := false
	for index, raw := range copied {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if message["role"] != "system" {
			continue
		}
		clone := map[string]any{}
		for key, value := range message {
			clone[key] = value
		}
		if text, ok := clone["content"].(string); ok {
			clone["content"] = text + "\n\n" + prompt
		}
		copied[index] = clone
		added = true
		break
	}
	if !added {
		copied = append([]any{map[string]any{"role": "system", "content": prompt}}, copied...)
	}
	result["messages"] = copied
	result["response_format"] = map[string]any{"type": "json_object"}
	return result
}
