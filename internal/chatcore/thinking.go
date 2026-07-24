package chatcore

func NormalizeThinking(body map[string]any, mode string) map[string]any {
	result := map[string]any{}
	for key, value := range body {
		result[key] = value
	}
	if mode != "" && mode != "auto" {
		if _, exists := result["thinking"]; !exists && result["reasoning_effort"] == nil {
			switch mode {
			case "on":
				result["thinking"] = map[string]any{"type": "enabled", "budget_tokens": 10000}
			case "none", "low", "medium", "high":
				result["reasoning_effort"] = mode
			}
		}
	}
	if messages, ok := result["messages"].([]any); ok && len(messages) > 0 {
		last, _ := messages[len(messages)-1].(map[string]any)
		if last["role"] != "user" {
			delete(result, "thinking")
		}
	}
	return result
}
