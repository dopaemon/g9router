package server

import (
	"io"
	"net/http"
	"strings"
)

func (s *Server) openCodeChat(w http.ResponseWriter, r *http.Request, body []byte, providerID string) bool {
	endpoint := "https://opencode.ai/zen/v1/chat/completions"
	if providerID == "opencode-go" {
		endpoint = "https://opencode.ai/zen/go/v1/chat/completions"
	}
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
