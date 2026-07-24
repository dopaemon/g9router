package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var grokTurnState sync.Map

func (s *Server) grokCLIResponses(w http.ResponseWriter, incoming *http.Request, body []byte, accessToken string, providerData map[string]any) bool {
	var request map[string]any
	if json.Unmarshal(body, &request) != nil || accessToken == "" {
		return false
	}
	model, _ := request["model"].(string)
	model = strings.TrimSuffix(model, "-low")
	model = strings.TrimSuffix(model, "-medium")
	model = strings.TrimSuffix(model, "-high")
	request["model"] = model
	request["stream"] = true
	request["store"] = false
	delete(request, "previous_response_id")
	delete(request, "max_tokens")
	delete(request, "max_completion_tokens")
	delete(request, "messages")
	if _, ok := request["reasoning"]; !ok {
		request["reasoning"] = map[string]any{"summary": "concise", "effort": "high"}
	}
	if reasoning, ok := request["reasoning"].(map[string]any); ok && reasoning["summary"] == nil {
		reasoning["summary"] = "concise"
	}
	include, _ := request["include"].([]any)
	seen := false
	for _, value := range include {
		if value == "reasoning.encrypted_content" {
			seen = true
		}
	}
	if !seen {
		include = append(include, "reasoning.encrypted_content")
		request["include"] = include
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return false
	}
	session := stringValue(providerData["sessionId"])
	if session == "" {
		session = stringValue(providerData["email"])
	}
	if session == "" {
		session = grokRandomID()
	}
	turn := 1
	if previous, ok := grokTurnState.Load(session); ok {
		turn = previous.(int) + 1
	}
	grokTurnState.Store(session, turn)
	requestID := grokRandomID()
	endpoint := "https://cli-chat-proxy.grok.com/v1/responses"
	upstream, err := http.NewRequestWithContext(incoming.Context(), http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	for key, value := range map[string]string{
		"Content-Type": "application/json", "Accept": "text/event-stream", "Authorization": "Bearer " + accessToken,
		"x-grok-client-identifier": "grok-cli", "x-grok-client-version": "1.0.0",
		"x-grok-session-id": session, "x-grok-conv-id": session, "x-grok-req-id": requestID,
		"x-grok-turn-idx": stringInt(turn), "x-grok-model-override": model,
	} {
		upstream.Header.Set(key, value)
	}
	if email := stringValue(providerData["email"]); email != "" {
		upstream.Header.Set("x-email", email)
	}
	if userID := stringValue(providerData["userId"]); userID != "" {
		upstream.Header.Set("x-userid", userID)
	}
	response, err := s.client.Do(upstream)
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

func grokRandomID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(value)
}
func stringInt(value int) string { return fmt.Sprintf("%d", value) }
