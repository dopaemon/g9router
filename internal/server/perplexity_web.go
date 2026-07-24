package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		content := stringValue(message["content"])
		if content == "" {
			continue
		}
		if message["role"] == "system" {
			query += content + "\n"
		} else {
			query += content + "\n"
		}
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return false
	}
	frontend := uuidToken(query + fmt.Sprint(time.Now().UnixNano()))
	payload := map[string]any{"query_str": query, "params": map[string]any{"query_str": query, "search_focus": "internet", "mode": mode, "model_preference": preference, "sources": []string{"web"}, "attachments": []any{}, "frontend_uuid": frontend, "frontend_context_uuid": uuidToken(frontend), "version": "2.18", "language": "en-US", "timezone": "UTC", "search_recency_filter": nil, "is_incognito": true, "use_schematized_api": true, "last_backend_uuid": nil}}
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
		text := perplexityBlockText(event)
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
		text = cleanPerplexityText(text)
		if stream && text != "" {
			chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": nil}}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
	}
	if stream {
		chunk, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "object": "chat.completion", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": cleanPerplexityText(full)}, "finish_reason": "stop"}}})
	return true
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
	value = strings.ReplaceAll(value, "[1]", "")
	value = strings.ReplaceAll(value, "[2]", "")
	return strings.TrimSpace(value)
}
