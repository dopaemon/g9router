package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *Server) ollamaChatAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORS(w, "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
		return
	}
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	model, _ := input["model"].(string)
	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	s.chatCompletions(recorder, clone)
	if recorder.Code < 200 || recorder.Code >= 300 {
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
		return
	}
	if strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream") || input["stream"] == true {
		setCORS(w, "GET, POST, OPTIONS")
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, line := range strings.Split(recorder.Body.String(), "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				writeNDJSON(w, map[string]any{"model": model, "message": map[string]any{"role": "assistant", "content": ""}, "done": true})
				continue
			}
			var chunk map[string]any
			if json.Unmarshal([]byte(data), &chunk) != nil {
				continue
			}
			choices, _ := chunk["choices"].([]any)
			if len(choices) == 0 {
				continue
			}
			choice, _ := choices[0].(map[string]any)
			delta, _ := choice["delta"].(map[string]any)
			content, _ := delta["content"].(string)
			writeNDJSON(w, map[string]any{"model": model, "message": map[string]any{"role": "assistant", "content": content}, "done": choice["finish_reason"] != nil})
		}
		return
	}
	var completion map[string]any
	if json.Unmarshal(recorder.Body.Bytes(), &completion) != nil {
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
		return
	}
	choices, _ := completion["choices"].([]any)
	message := map[string]any{"role": "assistant", "content": ""}
	if len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if value, ok := choice["message"].(map[string]any); ok {
				message = value
			}
		}
	}
	writeJSON(w, recorder.Code, map[string]any{"model": model, "message": message, "done": true})
}

func writeNDJSON(w http.ResponseWriter, value any) {
	data, _ := json.Marshal(value)
	_, _ = w.Write(append(data, '\n'))
}
