package server

import (
	"encoding/json"
	"strings"
)

func prepareKimchiBody(body map[string]any) []byte {
	next := cloneMap(body)
	if system := kimchiSystemText(next["system"]); system != "" {
		messages, _ := next["messages"].([]any)
		found := false
		for _, raw := range messages {
			message, ok := raw.(map[string]any)
			if !ok || message["role"] != "system" {
				continue
			}
			if content, ok := message["content"].(string); ok {
				message["content"] = system + "\n\n" + content
			} else {
				message["content"] = system
			}
			found = true
			break
		}
		if !found {
			messages = append([]any{map[string]any{"role": "system", "content": system}}, messages...)
		}
		next["messages"] = messages
	}
	delete(next, "system")
	for _, key := range []string{"anthropic_version", "anthropic_beta", "client_metadata", "mcp_servers", "stop_sequences", "thinking", "top_k"} {
		delete(next, key)
	}
	if messages, ok := next["messages"].([]any); ok {
		for _, raw := range messages {
			if message, ok := raw.(map[string]any); ok {
				delete(message, "cache_control")
				kimchiCleanParts(message)
			}
		}
	}
	if tools, ok := next["tools"].([]any); ok {
		for _, raw := range tools {
			if tool, ok := raw.(map[string]any); ok {
				delete(tool, "cache_control")
			}
		}
	}
	return mustJSON(next)
}

func kimchiSystemText(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	parts, _ := value.([]any)
	texts := make([]string, 0, len(parts))
	for _, raw := range parts {
		if item, ok := raw.(map[string]any); ok {
			if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
				texts = append(texts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

func kimchiCleanParts(message map[string]any) {
	parts, ok := message["content"].([]any)
	if !ok {
		return
	}
	for _, raw := range parts {
		if part, ok := raw.(map[string]any); ok {
			delete(part, "cache_control")
			delete(part, "signature")
		}
	}
}

func mustJSON(value any) []byte { encoded, _ := json.Marshal(value); return encoded }
