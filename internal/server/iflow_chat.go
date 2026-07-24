package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) proxyIFlow(w http.ResponseWriter, incoming *http.Request, baseURL string, body map[string]any, apiKey string) bool {
	if apiKey == "" {
		return false
	}
	stream, _ := body["stream"].(bool)
	if stream && body["stream_options"] == nil {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return false
	}
	session := "session-" + uuid.NewString()
	timestamp := time.Now().UnixMilli()
	userAgent := "iFlow-Cli"
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(userAgent + ":" + session + ":" + stringInt64(timestamp)))
	request, err := http.NewRequestWithContext(incoming.Context(), http.MethodPost, baseURL, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	for key, value := range map[string]string{"Content-Type": "application/json", "User-Agent": userAgent, "session-id": session, "x-iflow-timestamp": stringInt64(timestamp), "x-iflow-signature": hex.EncodeToString(mac.Sum(nil)), "Authorization": "Bearer " + apiKey} {
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

func stringInt64(value int64) string { return strconv.FormatInt(value, 10) }
