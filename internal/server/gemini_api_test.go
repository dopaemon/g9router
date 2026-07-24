package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestGeminiModelResourceEndToEnd(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"model":"gemini/test","choices":[{"message":{"content":"pong"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	app := New(Options{Upstream: upstream.URL, ProviderPath: t.TempDir() + "/providers.json", OAuthPath: os.TempDir() + "/gemini-test-oauth.json"})
	request := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini/test:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"ping"}]}]}`))
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"candidates"`) || !strings.Contains(response.Body.String(), "pong") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
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

func TestGeminiStreamMapping(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeGeminiStream(recorder, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"), "gemini/x")
	if recorder.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(recorder.Body.String(), `"candidates"`) || strings.Contains(recorder.Body.String(), "[DONE]") {
		t.Fatalf("unexpected stream: headers=%v body=%s", recorder.Header(), recorder.Body.String())
	}
}
