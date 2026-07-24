package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

func (s *Server) modelTestAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Model string `json:"model"`
		Kind  string `json:"kind"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Model) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Model required"})
		return
	}
	if input.Kind == "" {
		input.Kind = "llm"
	}
	path := "/v1/chat/completions"
	payload := map[string]any{"model": input.Model, "max_tokens": 16, "stream": false, "messages": []any{map[string]any{"role": "user", "content": "hi"}}}
	handler := s.chatCompletions
	if input.Kind == "embedding" {
		path = "/v1/embeddings"
		payload = map[string]any{"model": input.Model, "input": "test"}
		handler = s.embeddings
	}
	body, _ := json.Marshal(payload)
	probe := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	probe.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	started := time.Now()
	handler(recorder, probe)
	latency := time.Since(started).Milliseconds()
	if recorder.Code < 200 || recorder.Code >= 300 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latencyMs": latency, "status": recorder.Code, "error": compactProbeError(recorder.Body.String())})
		return
	}
	var result map[string]any
	if json.Unmarshal(recorder.Body.Bytes(), &result) != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latencyMs": latency, "status": recorder.Code, "error": "Provider returned invalid JSON"})
		return
	}
	if input.Kind == "embedding" {
		data, _ := result["data"].([]any)
		if len(data) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latencyMs": latency, "status": recorder.Code, "error": "Provider returned no embedding data"})
			return
		}
	} else {
		choices, _ := result["choices"].([]any)
		if len(choices) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latencyMs": latency, "status": recorder.Code, "error": "Provider returned no completion choices"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "latencyMs": latency, "status": recorder.Code, "error": nil})
}

func compactProbeError(raw string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) == nil {
		if errValue, ok := payload["error"].(map[string]any); ok {
			if message, ok := errValue["message"].(string); ok {
				return message
			}
		}
		if message, ok := payload["error"].(string); ok {
			return message
		}
	}
	return strings.TrimSpace(raw)
}
