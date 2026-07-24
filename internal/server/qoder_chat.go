package server

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const qoderChatURL = "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
const qoderModelListURL = "https://api3.qoder.sh/algo/api/v2/model/list"

func (s *Server) qoderChat(w http.ResponseWriter, r *http.Request, body []byte, accessToken string, providerData map[string]any) bool {
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		return false
	}
	model, _ := input["model"].(string)
	if strings.HasPrefix(model, "qoder/") {
		model = strings.TrimPrefix(model, "qoder/")
	}
	messages, system := qoderMessages(input["messages"])
	lastUser := ""
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index]["role"] == "user" {
			lastUser, _ = messages[index]["content"].(string)
			break
		}
	}
	maxTokens := 32768
	for _, key := range []string{"max_tokens", "max_completion_tokens"} {
		if value, ok := input[key].(float64); ok && value > 0 && int(value) < maxTokens {
			maxTokens = int(value)
		}
	}
	userID := stringValue(providerData["userId"])
	if userID == "" || accessToken == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"message": "qoder credential is missing userId or accessToken; reconnect the account"}})
		return true
	}
	modelConfig, err := s.qoderModelConfig(r, accessToken, providerData, model)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]string{"message": err.Error()}})
		return true
	}
	recordID := qoderRecordID(model, messages, input["tools"], maxTokens)
	sessionID := qoderHash("qoder-session", userID, model)
	payload := map[string]any{
		"request_id": uuid.NewString(), "request_set_id": recordID, "chat_record_id": recordID,
		"session_id": sessionID, "stream": true, "chat_task": "FREE_INPUT", "is_reply": true,
		"is_retry": false, "source": 1, "version": "3", "session_type": "qodercli",
		"agent_id": "agent_common", "task_id": "common", "code_language": "", "chat_prompt": "",
		"image_urls": nil, "aliyun_user_type": "", "system": system, "messages": messages,
		"tools": qoderTools(input["tools"]), "parameters": map[string]int{"max_tokens": maxTokens},
		"chat_context": map[string]any{"chatPrompt": "", "imageUrls": nil, "extra": map[string]any{"context": []any{}, "modelConfig": map[string]any{"key": model, "is_reasoning": modelConfig["is_reasoning"]}, "originalContent": lastUser}, "features": []any{}, "text": lastUser},
		"model_config": modelConfig,
		"business":     map[string]any{"product": "cli", "version": "1.0.0", "type": "agent", "stage": "start", "id": uuid.NewString(), "name": truncateQoder(lastUser, 30), "begin_at": time.Now().UnixMilli()},
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	encoded := qoderEncodeBody(plain)
	psd := qoderCOSYCredentials{UserID: userID, AuthToken: accessToken, Name: stringValue(providerData["name"]), Email: stringValue(providerData["email"]), MachineID: stringValue(providerData["machineId"])}
	headers, err := qoderCOSYHeaders(encoded, qoderChatURL, psd)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"message": err.Error()}})
		return true
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, qoderChatURL, bytes.NewReader(encoded))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("X-Model-Key", model)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
		return true
	}
	wrapped := qoderUnwrapSSE(response.Body, "qoder/"+model)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(wrapped)
	return true
}

func (s *Server) qoderModelConfig(r *http.Request, token string, providerData map[string]any, model string) (map[string]any, error) {
	credentials := qoderCOSYCredentials{UserID: stringValue(providerData["userId"]), AuthToken: token, Name: stringValue(providerData["name"]), Email: stringValue(providerData["email"]), MachineID: stringValue(providerData["machineId"])}
	headers, err := qoderCOSYHeaders(nil, qoderModelListURL, credentials)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, qoderModelListURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Encoding", "identity")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("qoder model list failed: %s", response.Status)
	}
	var payload struct {
		Chat []map[string]any `json:"chat"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	for _, config := range payload.Chat {
		if stringValue(config["key"]) == model {
			return config, nil
		}
	}
	return nil, fmt.Errorf("qoder model_config for %q not found", model)
}

func qoderMessages(raw any) ([]map[string]any, string) {
	items, _ := raw.([]any)
	result := make([]map[string]any, 0, len(items))
	systems := make([]string, 0)
	for _, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		content := qoderText(message["content"])
		if role == "system" {
			if content != "" {
				systems = append(systems, content)
			}
			continue
		}
		result = append(result, map[string]any{"role": role, "content": content})
	}
	return result, strings.Join(systems, "\n\n")
}

func qoderText(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	if parts, ok := value.([]any); ok {
		texts := make([]string, 0, len(parts))
		for _, part := range parts {
			if item, ok := part.(map[string]any); ok {
				if text, ok := item["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "\n")
	}
	return fmt.Sprint(value)
}
func qoderTools(value any) []any {
	tools, _ := value.([]any)
	if tools == nil {
		return []any{}
	}
	return tools
}
func truncateQoder(value string, limit int) string {
	if len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}
func qoderHash(prefix string, values ...string) string {
	hash := sha256.New()
	hash.Write([]byte(prefix))
	for _, value := range values {
		hash.Write([]byte{0})
		hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))[:16]
}
func qoderRecordID(model string, messages []map[string]any, tools any, maxTokens int) string {
	data, _ := json.Marshal([]any{model, messages, tools, maxTokens})
	return qoderHash("qoder-record", string(data))
}

func qoderUnwrapSSE(body io.Reader, model string) []byte {
	var output bytes.Buffer
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	done := false
	for scanner.Scan() && !done {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			output.WriteString("data: [DONE]\n\n")
			break
		}
		var envelope struct {
			Status int    `json:"statusCodeValue"`
			Body   string `json:"body"`
		}
		if json.Unmarshal([]byte(data), &envelope) != nil {
			continue
		}
		if envelope.Status != 0 && envelope.Status != 200 {
			done = true
			continue
		}
		if envelope.Body == "[DONE]" {
			output.WriteString("data: [DONE]\n\n")
			break
		}
		if envelope.Body != "" {
			output.WriteString("data: ")
			output.WriteString(strings.ReplaceAll(strings.ReplaceAll(envelope.Body, "\r", ""), "\n", ""))
			output.WriteString("\n\n")
		}
	}
	if !strings.HasSuffix(output.String(), "data: [DONE]\n\n") {
		output.WriteString("data: [DONE]\n\n")
	}
	return output.Bytes()
}
