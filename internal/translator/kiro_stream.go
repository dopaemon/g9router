package translator

import (
	"encoding/json"
	"fmt"
	"time"
)

type KiroStreamState struct {
	ResponseID string
	Model      string
	Index      int
	HadToolUse bool
	Usage      map[string]any
	Finished   bool
}

func KiroEventToOpenAISSE(eventType string, raw []byte, state *KiroStreamState) []string {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	if state.ResponseID == "" {
		state.ResponseID = "chatcmpl-" + fmt.Sprint(time.Now().UnixNano())
		state.Model = "kiro"
	}
	chunk := func(delta map[string]any, finish any) string {
		return kiroSSE(map[string]any{"id": state.ResponseID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": state.Model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
	}
	if nested := value[eventType]; nested != nil {
		if object, ok := nested.(map[string]any); ok {
			value = object
		}
	}
	switch eventType {
	case "assistantResponseEvent":
		if text, _ := value["content"].(string); text != "" {
			state.Index++
			return []string{chunk(map[string]any{"content": text}, nil)}
		}
	case "reasoningContentEvent":
		text, _ := value["content"].(string)
		if text == "" {
			text, _ = value["text"].(string)
		}
		if text != "" {
			state.Index++
			return []string{chunk(map[string]any{"reasoning_content": text}, nil)}
		}
	case "toolUseEvent":
		state.HadToolUse = true
		id, _ := value["toolUseId"].(string)
		name, _ := value["name"].(string)
		input := value["input"]
		encoded, _ := json.Marshal(input)
		state.Index++
		return []string{chunk(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}}}, nil)}
	case "usageEvent":
		state.Usage = value
		return nil
	case "messageStopEvent", "done":
		if state.Finished {
			return nil
		}
		state.Finished = true
		reason := "stop"
		if state.HadToolUse {
			reason = "tool_calls"
		}
		output := chunk(map[string]any{}, reason)
		if state.Usage != nil {
			output = addKiroUsage(output, state.Usage)
		}
		return []string{output}
	}
	return nil
}

func kiroSSE(value any) string {
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("data: %s\n\n", encoded)
}
func addKiroUsage(raw string, usage map[string]any) string {
	var value map[string]any
	payload := raw[6:]
	if json.Unmarshal([]byte(payload), &value) != nil {
		return raw
	}
	value["usage"] = usage
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("data: %s\n\n", encoded)
}
