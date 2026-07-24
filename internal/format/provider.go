package format

import "strings"

const (
	OpenAI    = "openai"
	Claude    = "claude"
	Gemini    = "gemini"
	Responses = "openai-responses"
)

func Detect(body map[string]any) string {
	if input, ok := body["input"]; ok && !has(body, "messages") {
		switch input.(type) {
		case string, []any:
			return Responses
		}
	}
	if contents, ok := body["contents"].([]any); ok && contents != nil {
		return Gemini
	}
	if body["system"] != nil || body["anthropic_version"] != nil {
		return Claude
	}
	if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
		if first, ok := messages[0].(map[string]any); ok {
			if parts, ok := first["content"].([]any); ok {
				for _, raw := range parts {
					part, _ := raw.(map[string]any)
					if part["type"] == "tool_use" || part["type"] == "tool_result" {
						return Claude
					}
					if part["type"] == "image" {
						return Claude
					}
				}
			}
		}
	}
	return OpenAI
}
func Target(provider string) string {
	if strings.HasPrefix(provider, "anthropic-compatible-") {
		return Claude
	}
	if strings.HasPrefix(provider, "openai-compatible-") && strings.Contains(provider, "responses") {
		return Responses
	}
	if strings.HasPrefix(provider, "gemini") {
		return Gemini
	}
	return OpenAI
}
func has(body map[string]any, key string) bool { _, ok := body[key]; return ok }
