package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

func (s *Server) geminiModelAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1beta/models/")
	path = strings.TrimPrefix(path, "/v1beta/models/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "Missing model", "type": "invalid_request_error"}})
		return
	}
	modelAction := parts[len(parts)-1]
	modelAction = strings.TrimSuffix(modelAction, ":streamGenerateContent")
	modelAction = strings.TrimSuffix(modelAction, ":generateContent")
	model := modelAction
	if len(parts) > 1 {
		model = parts[len(parts)-2] + "/" + modelAction
	}
	stream := strings.Contains(parts[len(parts)-1], ":streamGenerateContent")
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
		return
	}
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	converted := geminiRequest(input, model, stream)
	encoded, _ := json.Marshal(converted)
	request := r.Clone(r.Context())
	request.URL.Path = "/v1/chat/completions"
	request.Body = io.NopCloser(bytes.NewReader(encoded))
	request.ContentLength = int64(len(encoded))
	response := httptest.NewRecorder()
	s.chatCompletions(response, request)
	if response.Code >= 400 {
		copyResponse(w, response)
		return
	}
	if stream {
		writeGeminiStream(w, response.Body.Bytes(), model)
		return
	}
	writeGeminiResponse(w, response.Body.Bytes(), model)
}

func geminiRequest(input map[string]any, model string, stream bool) map[string]any {
	messages := []map[string]any{}
	if instruction, ok := input["systemInstruction"].(map[string]any); ok {
		if text := geminiPartsText(instruction["parts"]); text != "" {
			messages = append(messages, map[string]any{"role": "system", "content": text})
		}
	}
	if contents, ok := input["contents"].([]any); ok {
		for _, raw := range contents {
			content, _ := raw.(map[string]any)
			role := "user"
			if content["role"] == "model" {
				role = "assistant"
			}
			messages = append(messages, map[string]any{"role": role, "content": geminiPartsText(content["parts"])})
		}
	}
	result := map[string]any{"model": model, "messages": messages, "stream": stream}
	if config, ok := input["generationConfig"].(map[string]any); ok {
		for source, target := range map[string]string{"maxOutputTokens": "max_tokens", "temperature": "temperature", "topP": "top_p"} {
			if value, exists := config[source]; exists {
				result[target] = value
			}
		}
	}
	return result
}

func geminiPartsText(raw any) string {
	parts, _ := raw.([]any)
	values := make([]string, 0, len(parts))
	for _, item := range parts {
		part, _ := item.(map[string]any)
		if text, ok := part["text"].(string); ok {
			values = append(values, text)
		}
	}
	return strings.Join(values, "\n")
}

func writeGeminiResponse(w http.ResponseWriter, raw []byte, model string) {
	var input map[string]any
	if json.Unmarshal(raw, &input) != nil {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write(raw)
		return
	}
	if _, ok := input["error"]; ok {
		writeJSON(w, http.StatusBadGateway, input)
		return
	}
	choices, _ := input["choices"].([]any)
	if len(choices) == 0 {
		writeJSON(w, http.StatusOK, input)
		return
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	text, _ := message["content"].(string)
	finish, _ := choice["finish_reason"].(string)
	if finish == "" {
		finish = "STOP"
	} else if finish == "stop" {
		finish = "STOP"
	} else if finish == "length" {
		finish = "MAX_TOKENS"
	}
	result := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": text}}}, "finishReason": finish, "index": 0}}, "modelVersion": model}
	if usage, ok := input["usage"].(map[string]any); ok {
		result["usageMetadata"] = map[string]any{"promptTokenCount": usage["prompt_tokens"], "candidatesTokenCount": usage["completion_tokens"], "totalTokenCount": usage["total_tokens"]}
	}
	writeJSON(w, http.StatusOK, result)
}

func writeGeminiStream(w http.ResponseWriter, raw []byte, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var input map[string]any
		if json.Unmarshal([]byte(data), &input) != nil {
			continue
		}
		choices, _ := input["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		text, _ := delta["content"].(string)
		candidate := map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": text}}}, "index": 0}
		if finish, _ := choice["finish_reason"].(string); finish != "" {
			candidate["finishReason"] = map[string]string{"stop": "STOP", "length": "MAX_TOKENS"}[finish]
			if candidate["finishReason"] == "" {
				candidate["finishReason"] = "STOP"
			}
		}
		encoded, _ := json.Marshal(map[string]any{"candidates": []any{candidate}, "modelVersion": model})
		_, _ = w.Write([]byte("data: " + string(encoded) + "\r\n\r\n"))
	}
}

func copyResponse(w http.ResponseWriter, response *httptest.ResponseRecorder) {
	for key, values := range response.Header() {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.Code)
	_, _ = w.Write(response.Body.Bytes())
}
