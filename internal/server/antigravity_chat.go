package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
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
	wrapped := map[string]any{"project": project, "model": model, "userAgent": "antigravity", "requestType": "agent", "requestId": requestID, "request": body}
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
	_, _ = io.Copy(w, response.Body)
	return response.StatusCode < 500
}

func antigravityRequestID(providerData map[string]any, model string) string {
	seed := stringValue(providerData["email"]) + ":" + model
	sum := sha256.Sum256([]byte(seed))
	conversation := hex.EncodeToString(sum[:16])
	return "agent/" + conversation + "/" + time.Now().Format("20060102150405") + "/" + conversation + "/1"
}
