package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiRequestMapping(t *testing.T) {
	request := geminiRequest(map[string]any{
		"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": "be concise"}}},
		"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "hello"}}}},
		"generationConfig":  map[string]any{"maxOutputTokens": 12.0, "topP": 0.8},
	}, "gemini/gemini-2.5-pro", false)
	if request["model"] != "gemini/gemini-2.5-pro" || request["stream"] != false {
		t.Fatalf("unexpected request: %#v", request)
	}
	messages := request["messages"].([]map[string]any)
	if len(messages) != 2 || messages[0]["role"] != "system" || messages[1]["content"] != "hello" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestGeminiResponseMapping(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeGeminiResponse(recorder, []byte(`{"model":"x","choices":[{"message":{"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`), "gemini/x")
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, ok := response["candidates"]; !ok || !strings.Contains(recorder.Body.String(), "hello") {
		t.Fatalf("unexpected Gemini response: %s", recorder.Body.String())
	}
}
