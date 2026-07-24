package translator

func OpenAIToVertex(model string, body map[string]any) map[string]any {
	result := OpenAIToGemini(model, body)
	contents, _ := result["contents"].([]any)
	for _, raw := range contents {
		turn, _ := raw.(map[string]any)
		parts, _ := turn["parts"].([]any)
		for _, partRaw := range parts {
			part, _ := partRaw.(map[string]any)
			if call, ok := part["functionCall"].(map[string]any); ok {
				delete(call, "id")
				call["thoughtSignature"] = "B####"
			}
			if response, ok := part["functionResponse"].(map[string]any); ok {
				delete(response, "id")
			}
		}
	}
	return result
}
