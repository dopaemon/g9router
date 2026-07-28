package translator

import "testing"

func BenchmarkOpenAIToGemini(b *testing.B) {
	body := map[string]any{
		"system": "You are a helpful assistant.",
		"messages": []any{
			map[string]any{"role": "user", "content": "Explain this request."},
			map[string]any{"role": "assistant", "content": "I can help."},
		},
		"temperature": 0.2,
		"max_tokens":  512,
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": "read_file", "description": "Read a file", "parameters": map[string]any{"type": "object"},
		}}},
	}
	b.ReportAllocs()
	for b.Loop() {
		OpenAIToGemini("gemini-2.5-pro", body)
	}
}

func BenchmarkNormalizeCodexRequest(b *testing.B) {
	body := map[string]any{
		"model":        "gpt-5.4",
		"instructions": "You are a helpful assistant.",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "Hello"},
			"resp_old",
		},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": "read_file", "description": "Read a file", "parameters": map[string]any{"type": "object"},
		}}},
		"tool_choice": map[string]any{"type": "function", "name": "read_file"},
	}
	b.ReportAllocs()
	for b.Loop() {
		NormalizeCodexRequest(body)
	}
}
