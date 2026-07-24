package chatcore

func FixMissingToolResponses(messages []any) []any {
	result := make([]any, len(messages))
	copy(result, messages)
	for i := 0; i < len(result); i++ {
		assistant, ok := result[i].(map[string]any)
		if !ok || assistant["role"] != "assistant" {
			continue
		}
		calls, _ := assistant["tool_calls"].([]any)
		if len(calls) == 0 {
			continue
		}
		responded := map[string]bool{}
		insert := i + 1
		for insert < len(result) {
			message, _ := result[insert].(map[string]any)
			if message["role"] != "tool" {
				break
			}
			if id, ok := message["tool_call_id"].(string); ok {
				responded[id] = true
			}
			insert++
		}
		missing := []any{}
		for _, raw := range calls {
			call, _ := raw.(map[string]any)
			id, _ := call["id"].(string)
			if id != "" && !responded[id] {
				missing = append(missing, map[string]any{"role": "tool", "tool_call_id": id, "content": "[No response received]"})
			}
		}
		if len(missing) > 0 {
			tail := append([]any{}, result[insert:]...)
			result = append(result[:insert], missing...)
			result = append(result, tail...)
			i = insert + len(missing) - 1
		}
	}
	return result
}
