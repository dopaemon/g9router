package server

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type grokModelConfig struct {
	model, mode string
	thinking    bool
}

var grokModels = map[string]grokModelConfig{
	"grok-3": {"grok-3", "MODEL_MODE_GROK_3", false}, "grok-3-mini": {"grok-3", "MODEL_MODE_GROK_3_MINI_THINKING", true},
	"grok-3-thinking": {"grok-3", "MODEL_MODE_GROK_3_THINKING", true}, "grok-4": {"grok-4", "MODEL_MODE_GROK_4", false},
	"grok-4-mini": {"grok-4-mini", "MODEL_MODE_GROK_4_MINI_THINKING", true}, "grok-4-thinking": {"grok-4", "MODEL_MODE_GROK_4_THINKING", true},
	"grok-4-heavy": {"grok-4", "MODEL_MODE_HEAVY", true}, "grok-4.1-mini": {"grok-4-1-thinking-1129", "MODEL_MODE_GROK_4_1_MINI_THINKING", true},
	"grok-4.1-fast": {"grok-4-1-thinking-1129", "MODEL_MODE_FAST", false}, "grok-4.1-expert": {"grok-4-1-thinking-1129", "MODEL_MODE_EXPERT", true},
	"grok-4.1-thinking": {"grok-4-1-thinking-1129", "MODEL_MODE_GROK_4_1_THINKING", true}, "grok-4.2": {"grok-420", "MODEL_MODE_GROK_420", false},
	"grok-4.20": {"grok-420", "MODEL_MODE_GROK_420", false}, "grok-4.20-beta": {"grok-420", "MODEL_MODE_GROK_420", false},
}

func grokRandomHex(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return uuidToken(fmt.Sprint(time.Now().UnixNano()))[:size*2]
	}
	return fmt.Sprintf("%x", data)
}

func grokStatsigID() string {
	return base64.StdEncoding.EncodeToString([]byte("e:TypeError: Cannot read properties of undefined (reading 'grok')"))
}

func grokMessageContent(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	parts, ok := value.([]any)
	if !ok {
		return ""
	}
	var text []string
	for _, part := range parts {
		item, _ := part.(map[string]any)
		if stringValue(item["type"]) == "text" && stringValue(item["text"]) != "" {
			text = append(text, stringValue(item["text"]))
		}
	}
	return strings.Join(text, " ")
}

func (s *Server) grokWebChat(w http.ResponseWriter, r *http.Request, request map[string]any, token string) bool {
	messages, _ := request["messages"].([]any)
	if len(messages) == 0 || token == "" {
		return false
	}
	model := stringValue(request["model"])
	config, ok := grokModels[model]
	if !ok {
		config = grokModels["grok-4.1-fast"]
	}
	parts := make([]string, 0, len(messages))
	lastUser := -1
	for index, raw := range messages {
		message, _ := raw.(map[string]any)
		if stringValue(message["role"]) == "user" && strings.TrimSpace(grokMessageContent(message["content"])) != "" {
			lastUser = index
		}
	}
	for index, raw := range messages {
		message, _ := raw.(map[string]any)
		content := grokMessageContent(message["content"])
		if content == "" {
			continue
		}
		role := stringValue(message["role"])
		if role == "developer" {
			role = "system"
		}
		if role == "user" && index == lastUser {
			parts = append(parts, content)
		} else {
			parts = append(parts, role+": "+content)
		}
	}
	message := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if message == "" {
		return false
	}
	payload := map[string]any{"temporary": true, "modelName": config.model, "modelMode": config.mode, "message": message, "fileAttachments": []any{}, "imageAttachments": []any{}, "disableSearch": false, "enableImageGeneration": false, "returnImageBytes": false, "returnRawGrokInXaiRequest": false, "enableImageStreaming": false, "imageGenerationCount": 0, "forceConcise": false, "toolOverrides": map[string]any{}, "enableSideBySide": true, "sendFinalMetadata": true, "isReasoning": false, "disableTextFollowUps": false, "disableMemory": true, "forceSideBySide": false, "isAsyncChat": false, "disableSelfHarmShortCircuit": false, "deviceEnvInfo": map[string]any{"darkModeEnabled": false, "devicePixelRatio": 2, "screenWidth": 2056, "screenHeight": 1329, "viewportWidth": 2056, "viewportHeight": 1083}}
	encoded, _ := json.Marshal(payload)
	requestUpstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://grok.com/rest/app-chat/conversations/new", strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	requestUpstream.Header.Set("Content-Type", "application/json")
	requestUpstream.Header.Set("Accept", "*/*")
	requestUpstream.Header.Set("Origin", "https://grok.com")
	requestUpstream.Header.Set("Referer", "https://grok.com/")
	requestUpstream.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
	requestUpstream.Header.Set("Cache-Control", "no-cache")
	requestUpstream.Header.Set("Pragma", "no-cache")
	requestUpstream.Header.Set("x-statsig-id", grokStatsigID())
	requestUpstream.Header.Set("x-xai-request-id", grokRandomHex(16))
	requestUpstream.Header.Set("traceparent", "00-"+grokRandomHex(16)+"-"+grokRandomHex(8)+"-00")
	token = strings.TrimPrefix(token, "sso=")
	requestUpstream.Header.Set("Cookie", "sso="+token)
	response, err := s.client.Do(requestUpstream)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		if response != nil {
			response.Body.Close()
		}
		return false
	}
	defer response.Body.Close()
	stream, _ := request["stream"].(bool)
	id, created := "chatcmpl-grok-"+uuidToken(message)[:12], time.Now().Unix()
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
	}
	full := ""
	reasoning := ""
	fingerprint := ""
	responseID := ""
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 64<<20))
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(scanner.Text())), &event) != nil {
			continue
		}
		result, _ := event["result"].(map[string]any)
		resp, _ := result["response"].(map[string]any)
		if info, ok := resp["llmInfo"].(map[string]any); ok && fingerprint == "" {
			fingerprint = stringValue(info["modelHash"])
		}
		responseID = nonEmpty(stringValue(resp["responseId"]), responseID)
		if modelResponse, ok := resp["modelResponse"].(map[string]any); ok {
			if metadata, ok := modelResponse["metadata"].(map[string]any); ok {
				if info, ok := metadata["llm_info"].(map[string]any); ok {
					fingerprint = nonEmpty(stringValue(info["modelHash"]), fingerprint)
				}
			}
			if messageText := stringValue(modelResponse["message"]); messageText != "" {
				full = messageText
				if stream {
					grokWriteChunk(w, id, created, model, map[string]any{"content": messageText, "reasoning_content": nil}, fingerprint)
				}
				continue
			}
		}
		text := stringValue(resp["token"])
		if text == "" {
			continue
		}
		full += text
		if config.thinking && strings.HasPrefix(text, "<think>") {
			reasoning += strings.TrimPrefix(text, "<think>")
			continue
		}
		if stream {
			grokWriteChunk(w, id, created, model, map[string]any{"content": text}, fingerprint)
		}
	}
	if stream {
		chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
		return true
	}
	messageOut := map[string]any{"role": "assistant", "content": full}
	if reasoning != "" {
		messageOut["reasoning_content"] = reasoning
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "object": "chat.completion", "created": created, "model": model, "system_fingerprint": nonEmpty(fingerprint, ""), "choices": []any{map[string]any{"index": 0, "message": messageOut, "finish_reason": "stop"}}})
	return true
}

func grokWriteChunk(w http.ResponseWriter, id string, created int64, model string, delta map[string]any, fingerprint string) {
	chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "system_fingerprint": nonEmpty(fingerprint, ""), "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
}
