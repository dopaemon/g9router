package translator

func ChatToResponsesResponse(body map[string]any) map[string]any {
	output := []any{}
	choices := array(body["choices"])
	if len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		if text, ok := message["content"].(string); ok && text != "" {
			output = append(output, map[string]any{"type": "message", "id": body["id"], "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": text}}})
		}
		for _, raw := range array(message["tool_calls"]) {
			call, _ := raw.(map[string]any)
			function, _ := call["function"].(map[string]any)
			output = append(output, map[string]any{"type": "function_call", "call_id": call["id"], "name": function["name"], "arguments": function["arguments"]})
		}
	}
	return map[string]any{"id": body["id"], "object": "response", "model": body["model"], "output": output, "status": "completed", "usage": body["usage"]}
}
