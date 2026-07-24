package server

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type perplexitySession struct {
	backend string
	at      time.Time
}

var perplexitySessions = struct {
	sync.Mutex
	values map[string]perplexitySession
}{values: map[string]perplexitySession{}}

func (s *Server) perplexityWebChat(w http.ResponseWriter, r *http.Request, request map[string]any, token string) bool {
	messages, _ := request["messages"].([]any)
	if len(messages) == 0 || token == "" {
		return false
	}
	model := stringValue(request["model"])
	mode, preference := "copilot", model
	mapModel := map[string][2]string{"pplx-auto": {"concise", "pplx_pro"}, "pplx-sonar": {"copilot", "experimental"}, "pplx-gpt": {"copilot", "gpt54"}, "pplx-gemini": {"copilot", "gemini31pro_high"}, "pplx-sonnet": {"copilot", "claude46sonnet"}, "pplx-opus": {"copilot", "claude46opus"}, "pplx-nemotron": {"copilot", "nv_nemotron_3_super"}}
	if mapped, ok := mapModel[model]; ok {
		mode, preference = mapped[0], mapped[1]
	}
	query := ""
	history := make([]any, 0, len(messages))
	lastUser := -1
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		content := grokMessageContent(message["content"])
		if content == "" {
			continue
		}
		if message["role"] == "system" {
			query += content + "\n"
		} else {
			query += content + "\n"
			if stringValue(message["role"]) == "user" {
				lastUser = len(history)
			}
			history = append(history, map[string]string{"role": stringValue(message["role"]), "content": content})
		}
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return false
	}
	followUUID := perplexitySessionLookup(history[:maxInt(lastUser, 0)])
	if followUUID != "" && lastUser >= 0 {
		query = stringValue(history[lastUser].(map[string]string)["content"])
	}
	frontend := uuidToken(query + fmt.Sprint(time.Now().UnixNano()))
	payload := map[string]any{"query_str": query, "params": map[string]any{"query_str": query, "search_focus": "internet", "mode": mode, "model_preference": preference, "sources": []string{"web"}, "attachments": []any{}, "frontend_uuid": frontend, "frontend_context_uuid": uuidToken(frontend), "version": "2.18", "language": "en-US", "timezone": "UTC", "search_recency_filter": nil, "is_incognito": true, "use_schematized_api": true, "last_backend_uuid": nil}}
	if followUUID != "" {
		payload["params"].(map[string]any)["last_backend_uuid"] = followUUID
	}
	encoded, _ := json.Marshal(payload)
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://www.perplexity.ai/rest/sse/perplexity_ask", strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Accept", "text/event-stream")
	upstream.Header.Set("Cookie", "__Secure-next-auth.session-token="+token)
	upstream.Header.Set("Origin", "https://www.perplexity.ai")
	upstream.Header.Set("Referer", "https://www.perplexity.ai/")
	upstream.Header.Set("User-Agent", "Mozilla/5.0 Chrome/130.0.0.0")
	upstream.Header.Set("X-App-ApiClient", "default")
	upstream.Header.Set("X-App-ApiVersion", "2.18")
	response, err := s.client.Do(upstream)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		if response != nil {
			response.Body.Close()
		}
		return false
	}
	defer response.Body.Close()
	id, created := "chatcmpl-pplx-"+uuidToken(query)[:12], time.Now().Unix()
	stream, _ := request["stream"].(bool)
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
	}
	full := ""
	backendUUID := ""
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 64<<20))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
			continue
		}
		backendUUID = nonEmpty(stringValue(event["backend_uuid"]), backendUUID)
		text := perplexityBlockText(event)
		for _, thought := range perplexityThinking(event) {
			if stream {
				chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"reasoning_content": thought + "\n"}, "finish_reason": nil}}})
				_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			}
		}
		if text == "" {
			text = stringValue(event["text"])
		}
		if text == "" || strings.HasPrefix(text, "[DONE]") {
			continue
		}
		if strings.HasPrefix(text, full) {
			text = text[len(full):]
		}
		full += text
		text = cleanPerplexityDelta(text)
		if stream && text != "" {
			chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": nil}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
	}
	if stream {
		chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
		perplexitySessionStore(history[:maxInt(lastUser, 0)], query, cleanPerplexityText(full), backendUUID)
		return true
	}
	perplexitySessionStore(history[:maxInt(lastUser, 0)], query, cleanPerplexityText(full), backendUUID)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "object": "chat.completion", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": cleanPerplexityText(full)}, "finish_reason": "stop"}}})
	return true
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func perplexitySessionKey(history []any, current, answer string) string {
	data, _ := json.Marshal(append(append([]any{}, history...), map[string]string{"role": "user", "content": current}, map[string]string{"role": "assistant", "content": answer}))
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func perplexitySessionLookup(history []any) string {
	if len(history) == 0 {
		return ""
	}
	data, _ := json.Marshal(history)
	key := fmt.Sprintf("%x", sha256.Sum256(data))
	perplexitySessions.Lock()
	defer perplexitySessions.Unlock()
	entry, ok := perplexitySessions.values[key]
	if !ok || time.Since(entry.at) > time.Hour {
		return ""
	}
	return entry.backend
}

func perplexitySessionStore(history []any, current, answer, backend string) {
	if backend == "" {
		return
	}
	key := perplexitySessionKey(history, current, answer)
	perplexitySessions.Lock()
	defer perplexitySessions.Unlock()
	perplexitySessions.values[key] = perplexitySession{backend: backend, at: time.Now()}
}

func perplexityBlockText(event map[string]any) string {
	blocks, _ := event["blocks"].([]any)
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		if !strings.Contains(stringValue(block["intended_usage"]), "markdown") {
			continue
		}
		markdown, _ := block["markdown_block"].(map[string]any)
		chunks, _ := markdown["chunks"].([]any)
		result := make([]string, 0, len(chunks))
		for _, chunk := range chunks {
			result = append(result, stringValue(chunk))
		}
		return strings.Join(result, "")
	}
	return ""
}

func cleanPerplexityText(value string) string {
	return strings.TrimSpace(cleanPerplexityDelta(value))
}

func cleanPerplexityDelta(value string) string {
	value = regexp.MustCompile(`<\?xml[^?]*\?>|<grok:[^>]*>.*?</grok:[^>]*>|<grok:[^>]*/>|</?response\b[^>]*>`).ReplaceAllString(value, "")
	value = regexp.MustCompile(`\[\d+\]`).ReplaceAllString(value, "")
	return value
}

func perplexityThinking(event map[string]any) []string {
	blocks, _ := event["blocks"].([]any)
	var thoughts []string
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		if stringValue(block["intended_usage"]) != "pro_search_steps" && stringValue(block["intended_usage"]) != "plan" {
			continue
		}
		if plan, ok := block["plan_block"].(map[string]any); ok {
			steps, _ := plan["steps"].([]any)
			for _, rawStep := range steps {
				step, _ := rawStep.(map[string]any)
				if stringValue(step["step_type"]) == "SEARCH_WEB" {
					queries, _ := step["search_web_content"].(map[string]any)
					items, _ := queries["queries"].([]any)
					for _, rawQuery := range items {
						query, _ := rawQuery.(map[string]any)
						if value := stringValue(query["query"]); value != "" {
							thoughts = append(thoughts, "Searching: "+value)
						}
					}
				}
				if stringValue(step["step_type"]) == "READ_RESULTS" {
					results, _ := step["read_results_content"].(map[string]any)
					urls, _ := results["urls"].([]any)
					for index, rawURL := range urls {
						if index >= 3 {
							break
						}
						if value := stringValue(rawURL); value != "" {
							thoughts = append(thoughts, "Reading: "+value)
						}
					}
				}
			}
		}
		if goals, ok := block["plan_block"].(map[string]any); ok && stringValue(block["intended_usage"]) == "plan" {
			items, _ := goals["goals"].([]any)
			for _, rawGoal := range items {
				goal, _ := rawGoal.(map[string]any)
				if value := stringValue(goal["description"]); value != "" {
					thoughts = append(thoughts, value)
				}
			}
		}
	}
	return thoughts
}
