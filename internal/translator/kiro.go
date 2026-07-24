package translator

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

func OpenAIToKiro(model string, body map[string]any, stream bool) map[string]any {
	agentic := strings.HasSuffix(model, "-agentic")
	upstream := strings.TrimSuffix(model, "-agentic")
	history := []any{}
	var current map[string]any
	for _, raw := range array(body["messages"]) {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		content := textValue(message["content"])
		if role == "system" {
			content = "[System Instructions]\n" + content
			role = "user"
		}
		if role == "tool" {
			content = "[Tool result: " + content + "]"
			role = "user"
		}
		if role == "assistant" {
			for _, callRaw := range array(message["tool_calls"]) {
				call, _ := callRaw.(map[string]any)
				fn, _ := call["function"].(map[string]any)
				content += "\n[Tool call: " + stringValue(fn["name"]) + "(" + stringValue(fn["arguments"]) + ")]"
			}
			item := map[string]any{"assistantResponseMessage": map[string]any{"content": content}}
			history = append(history, item)
			continue
		}
		item := map[string]any{"userInputMessage": map[string]any{"content": content, "modelId": upstream, "origin": "AI_EDITOR"}}
		if current != nil {
			history = append(history, current)
		}
		current = item
	}
	if current == nil {
		current = map[string]any{"userInputMessage": map[string]any{"content": "continue", "modelId": upstream, "origin": "AI_EDITOR"}}
	}
	currentMessage := current["userInputMessage"].(map[string]any)
	currentMessage["content"] = "[Context: Current time is " + time.Now().UTC().Format(time.RFC3339Nano) + "]\n\n" + stringValue(currentMessage["content"])
	conversationID := kiroConversationID(body, history)
	continuationID := stringValue(body["agent_continuation_id"])
	if continuationID == "" {
		continuationID = conversationID
	}
	state := map[string]any{"chatTriggerType": "MANUAL", "conversationId": conversationID, "agentContinuationId": continuationID, "agentTaskType": "vibe", "currentMessage": current, "history": history}
	result := map[string]any{"conversationState": state, "agentMode": "vibe", "inferenceConfig": map[string]any{"maxTokens": 32000}}
	if agentic {
		result["systemPrompt"] = kiroAgenticSystemPrompt
	}
	if system, ok := body["system"].(string); ok && system != "" {
		result["systemPrompt"] = system
	}
	if value, ok := body["temperature"]; ok {
		result["inferenceConfig"].(map[string]any)["temperature"] = value
	}
	if value, ok := body["top_p"]; ok {
		result["inferenceConfig"].(map[string]any)["topP"] = value
	}
	_ = stream
	return result
}

func kiroConversationID(body map[string]any, history []any) string {
	for _, key := range []string{"conversation_id", "conversationId", "session_id", "sessionId"} {
		if value := stringValue(body[key]); value != "" {
			return value
		}
	}
	seed := stringValue(body["user_id"])
	if seed == "" {
		seed = stringValue(body["metadata"])
	}
	if seed == "" {
		seed = stringValue(body["model"]) + ":" + stringValue(body["system"])
	}
	if seed == "" {
		seed = stringValue(history)
	}
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:16])
}

const kiroAgenticSystemPrompt = "# CRITICAL: CHUNKED WRITE PROTOCOL (MANDATORY)\n\nYou MUST follow chunked write operations for file changes to avoid server timeouts."

func randomID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(raw)
}
