package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) openCodeChat(w http.ResponseWriter, r *http.Request, body []byte, providerID string) bool {
	endpoint := "https://opencode.ai/zen/v1/chat/completions"
	if providerID == "opencode-go" {
		endpoint = "https://opencode.ai/zen/go/v1/chat/completions"
	}
	body = injectOpenCodeReasoning(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer public")
	request.Header.Set("x-opencode-client", "desktop")
	request.Header.Set("Accept", "text/event-stream")
	response, err := s.client.Do(request)
	if err != nil || response.StatusCode >= 500 {
		if response != nil {
			response.Body.Close()
		}
		return false
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		if key == "Content-Length" || key == "Content-Encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
	return true
}

func injectOpenCodeReasoning(body []byte) []byte {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	model := stringValue(payload["model"])
	scope := ""
	if strings.HasPrefix(strings.ToLower(model), "kimi-") {
		scope = "toolCalls"
	}
	if strings.Contains(strings.ToLower(model), "deepseek") {
		scope = "all"
	}
	if scope == "" {
		return body
	}
	messages, _ := payload["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if stringValue(message["role"]) != "assistant" || stringValue(message["reasoning_content"]) != "" {
			continue
		}
		if scope == "toolCalls" {
			calls, _ := message["tool_calls"].([]any)
			if len(calls) == 0 {
				continue
			}
		}
		message["reasoning_content"] = " "
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return encoded
}
