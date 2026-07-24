package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) grokWebChat(w http.ResponseWriter, r *http.Request, request map[string]any, token string) bool {
	messages, _ := request["messages"].([]any)
	if len(messages) == 0 || token == "" {
		return false
	}
	model := stringValue(request["model"])
	modelName, modelMode := "grok-4.1-thinking-1129", "MODEL_MODE_FAST"
	if model == "grok-4" {
		modelName, modelMode = "grok-4", "MODEL_MODE_GROK_4"
	} else if model == "grok-3" {
		modelName, modelMode = "grok-3", "MODEL_MODE_GROK_3"
	} else if model == "grok-4-thinking" {
		modelName, modelMode = "grok-4", "MODEL_MODE_GROK_4_THINKING"
	}
	parts := make([]string, 0, len(messages))
	lastUser := -1
	for index, raw := range messages {
		message, _ := raw.(map[string]any)
		if stringValue(message["role"]) == "user" && stringValue(message["content"]) != "" {
			lastUser = index
		}
	}
	for index, raw := range messages {
		message, _ := raw.(map[string]any)
		content := stringValue(message["content"])
		if content == "" {
			continue
		}
		role := stringValue(message["role"])
		if role == "user" && index == lastUser {
			parts = append(parts, content)
		} else {
			parts = append(parts, role+": "+content)
		}
	}
	message := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if message == "" {
		return false
	}
	payload := map[string]any{"temporary": true, "modelName": modelName, "modelMode": modelMode, "message": message, "fileAttachments": []any{}, "imageAttachments": []any{}, "disableSearch": false, "enableImageGeneration": false, "returnImageBytes": false, "returnRawGrokInXaiRequest": false, "enableImageStreaming": false, "imageGenerationCount": 0, "forceConcise": false, "toolOverrides": map[string]any{}, "enableSideBySide": true, "sendFinalMetadata": true, "isReasoning": false, "disableTextFollowUps": false, "disableMemory": true, "forceSideBySide": false, "isAsyncChat": false}
	encoded, _ := json.Marshal(payload)
	requestUpstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://grok.com/rest/app-chat/conversations/new", strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	requestUpstream.Header.Set("Content-Type", "application/json")
	requestUpstream.Header.Set("Accept", "*/*")
	requestUpstream.Header.Set("Origin", "https://grok.com")
	requestUpstream.Header.Set("Referer", "https://grok.com/")
	requestUpstream.Header.Set("User-Agent", "Mozilla/5.0 Chrome/136.0.0.0")
	token = strings.TrimPrefix(token, "sso=")
	requestUpstream.Header.Set("Cookie", "sso="+token)
	response, err := s.client.Do(requestUpstream)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		if response != nil {
			response.Body.Close()
		}
		return false
	}
	defer response.Body.Close()
	stream, _ := request["stream"].(bool)
	id, created := "chatcmpl-grok-"+uuidToken(message)[:12], time.Now().Unix()
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
	}
	full := ""
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 64<<20))
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(scanner.Text())), &event) != nil {
			continue
		}
		result, _ := event["result"].(map[string]any)
		resp, _ := result["response"].(map[string]any)
		text := stringValue(resp["token"])
		fullMessage := false
		if text == "" {
			modelResponse, _ := resp["modelResponse"].(map[string]any)
			text = stringValue(modelResponse["message"])
			fullMessage = text != ""
		}
		if text == "" {
			continue
		}
		if fullMessage {
			full = text
		} else {
			full += text
		}
		if stream {
			chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": nil}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
	}
	if stream {
		chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "object": "chat.completion", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": full}, "finish_reason": "stop"}}})
	return true
}
