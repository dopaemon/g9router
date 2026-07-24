package translator

import "testing"

func TestGeminiTranslation(t *testing.T) {
	out := OpenAIToGemini("gemini", map[string]any{"system": "sys", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "temperature": 0.2})
	if out["systemInstruction"] == nil || len(out["contents"].([]any)) != 1 {
		t.Fatal(out)
	}
	openai := GeminiToOpenAI("gemini", map[string]any{"responseId": "r", "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "hi"}}}}}})
	if openai["choices"] == nil {
		t.Fatal(openai)
	}
}
