package server

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/translator"
)

func (s *Server) proxyAntigravity(w http.ResponseWriter, incoming *http.Request, baseURL, model string, body map[string]any, accessToken string, providerData map[string]any) bool {
	if accessToken == "" {
		return false
	}
	stream, _ := body["stream"].(bool)
	action := ":generateContent"
	if stream {
		action = ":streamGenerateContent?alt=sse"
	}
	project := stringValue(providerData["projectId"])
	requestID := antigravityRequestID(providerData, model)
	geminiBody := translator.OpenAIToGemini(model, body)
	wrapped := map[string]any{"project": project, "model": model, "userAgent": "antigravity", "requestType": "agent", "requestId": requestID, "request": geminiBody}
	encoded, err := json.Marshal(wrapped)
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(incoming.Context(), http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1internal"+action, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	for key, value := range map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + accessToken, "User-Agent": "antigravity", "Accept": "application/json"} {
		request.Header.Set(key, value)
	}
	if stream {
		request.Header.Set("Accept", "text/event-stream")
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		if key != "Content-Length" && key != "Content-Encoding" {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}
	w.WriteHeader(response.StatusCode)
	if stream {
		_, _ = io.WriteString(w, antigravitySSE(response.Body, model))
	} else {
		var payload map[string]any
		if json.NewDecoder(response.Body).Decode(&payload) != nil {
			return false
		}
		_ = json.NewEncoder(w).Encode(translator.GeminiToOpenAI(model, payload))
	}
	return response.StatusCode < 500
}

func antigravitySSE(body io.Reader, model string) string {
	var output strings.Builder
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &payload) != nil {
			continue
		}
		translated := translator.GeminiToOpenAI(model, payload)
		choices, _ := translated["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		message, _ := choice["message"].(map[string]any)
		content, _ := message["content"].(string)
		chunk := map[string]any{"id": translated["id"], "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": nil}}}
		encoded, _ := json.Marshal(chunk)
		output.WriteString("data: ")
		output.Write(encoded)
		output.WriteString("\n\n")
	}
	output.WriteString("data: [DONE]\n\n")
	return output.String()
}

func antigravityRequestID(providerData map[string]any, model string) string {
	seed := stringValue(providerData["email"]) + ":" + model
	sum := sha256.Sum256([]byte(seed))
	conversation := hex.EncodeToString(sum[:16])
	return "agent/" + conversation + "/" + time.Now().Format("20060102150405") + "/" + conversation + "/1"
}
