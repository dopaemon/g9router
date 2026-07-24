package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"g9router/internal/translator"
)

func (s *Server) proxyGeminiCLI(w http.ResponseWriter, incoming *http.Request, baseURL, model string, body map[string]any, accessToken string, providerData map[string]any) bool {
	if accessToken == "" {
		return false
	}
	stream, _ := body["stream"].(bool)
	action := ":generateContent"
	if stream {
		action = ":streamGenerateContent?alt=sse"
	}
	project := stringValue(providerData["projectId"])
	wrapped := map[string]any{"project": project, "model": model, "request": body}
	encoded, err := json.Marshal(wrapped)
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(incoming.Context(), http.MethodPost, strings.TrimRight(baseURL, "/")+action, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("User-Agent", "google-cloud-sdk gcloud/500.0.0")
	request.Header.Set("X-Goog-Api-Client", "gl-go/1.25.0 gccl/0.1.0")
	if stream {
		request.Header.Set("Accept", "text/event-stream")
	} else {
		request.Header.Set("Accept", "application/json")
	}
	response, err := s.client.Do(request)
	if err != nil {
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
