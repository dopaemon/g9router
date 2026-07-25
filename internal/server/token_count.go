package server

import (
	"encoding/json"
	"io"
	"net/http"
	"unicode/utf16"
)

func (s *Server) countTokensAPI(w http.ResponseWriter, r *http.Request) {
	setCORS(w, "POST, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var body map[string]any
	if json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&body) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}
	chars := countJSONValue(body["system"]) + countJSONValue(body["tools"])
	if messages, ok := body["messages"].([]any); ok {
		for _, message := range messages {
			item, _ := message.(map[string]any)
			chars += countMessageContent(item["content"])
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"input_tokens": (chars + 3) / 4})
}

func countMessageContent(value any) int {
	if blocks, ok := value.([]any); ok {
		total := 0
		for _, block := range blocks {
			if item, ok := block.(map[string]any); ok {
				switch item["type"] {
				case "text":
					total += countJSONValue(item["text"])
				case "tool_use":
					total += countJSONValue(item["name"]) + countJSONValue(item["input"])
				case "tool_result":
					total += countJSONValue(item["content"])
				case "thinking":
					total += countJSONValue(item["thinking"])
				default:
					total += countJSONValue(item)
				}
				continue
			}
			total += countJSONValue(block)
		}
		return total
	}
	return countJSONValue(value)
}

func countJSONValue(value any) int {
	switch item := value.(type) {
	case nil:
		return 0
	case string:
		return len(utf16.Encode([]rune(item)))
	case float64, bool:
		return len([]rune(toJSONString(item)))
	case []any:
		total := 0
		for _, child := range item {
			total += countJSONValue(child)
		}
		return total
	case map[string]any:
		total := 0
		for key, child := range item {
			total += len(utf16.Encode([]rune(key))) + countJSONValue(child)
		}
		return total
	default:
		return 0
	}
}

func toJSONString(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func setCORS(w http.ResponseWriter, methods string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", methods)
	w.Header().Set("Access-Control-Allow-Headers", "*")
}
