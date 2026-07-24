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

type commandCodeState struct {
	ID, Model string
	Created   int64
	Text      strings.Builder
}

func (s *Server) commandCodeChat(w http.ResponseWriter, r *http.Request, body []byte, apiKey string, request map[string]any) bool {
	if apiKey == "" {
		return false
	}
	request["stream"] = true
	encoded, _ := json.Marshal(request)
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.commandcode.ai/alpha/generate", strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Accept", "text/event-stream")
	upstream.Header.Set("Authorization", "Bearer "+apiKey)
	upstream.Header.Set("x-session-id", uuidToken(fmt.Sprintf("%d:%s", time.Now().UnixNano(), apiKey)))
	response, err := s.client.Do(upstream)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		if response != nil {
			response.Body.Close()
		}
		return false
	}
	defer response.Body.Close()
	model := stringValue(request["model"])
	state := commandCodeState{ID: "chatcmpl-" + uuidToken(model+fmt.Sprint(time.Now().UnixNano())), Model: model, Created: time.Now().Unix()}
	stream, _ := request["stream"].(bool)
	flusher, canFlush := w.(http.Flusher)
	if stream && canFlush {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 64<<20))
	for scanner.Scan() {
		event := strings.TrimSpace(scanner.Text())
		if event == "" {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(strings.TrimPrefix(event, "data:")), &payload) != nil {
			continue
		}
		for _, chunk := range commandCodeChunks(&state, payload) {
			if stream && canFlush {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
				flusher.Flush()
			}
		}
	}
	if stream && canFlush {
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": state.ID, "object": "chat.completion", "created": state.Created, "model": state.Model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": state.Text.String()}, "finish_reason": "stop"}}})
	return true
}

func commandCodeChunks(state *commandCodeState, event map[string]any) []string {
	eventType := stringValue(event["type"])
	text := stringValue(event["text"])
	if text == "" {
		text = stringValue(event["delta"])
	}
	if eventType == "text-delta" || eventType == "reasoning-delta" {
		if text == "" {
			return nil
		}
		state.Text.WriteString(text)
		delta := map[string]any{"content": text}
		if state.Text.Len() == len(text) {
			delta["role"] = "assistant"
		}
		return []string{commandCodeChunk(state, delta, nil)}
	}
	if eventType == "finish" || eventType == "finish-step" {
		return []string{commandCodeChunk(state, map[string]any{}, "stop")}
	}
	return nil
}

func commandCodeChunk(state *commandCodeState, delta map[string]any, finish *string) string {
	choice := map[string]any{"index": 0, "delta": delta, "finish_reason": finish}
	encoded, _ := json.Marshal(map[string]any{"id": state.ID, "object": "chat.completion.chunk", "created": state.Created, "model": state.Model, "choices": []any{choice}})
	return string(encoded)
}
