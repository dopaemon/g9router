package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) proxyQwen(w http.ResponseWriter, incoming *http.Request, baseURL string, body map[string]any, accessToken string, providerData map[string]any) bool {
	if accessToken == "" {
		return false
	}
	next := cloneMap(body)
	messages, _ := next["messages"].([]any)
	sentinel := map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": "", "cache_control": map[string]any{"type": "ephemeral"}}}}
	next["messages"] = append([]any{sentinel}, messages...)
	thinking := next["thinking"] != nil || next["enable_thinking"] == true
	if thinking {
		if choice := next["tool_choice"]; choice == "required" {
			next["tool_choice"] = "auto"
		} else if _, ok := choice.(map[string]any); ok {
			next["tool_choice"] = "auto"
		}
	}
	if stream, _ := next["stream"].(bool); stream && next["stream_options"] == nil && !thinking {
		next["stream_options"] = map[string]any{"include_usage": true}
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return false
	}
	resource := stringValue(providerData["resourceUrl"])
	if resource != "" {
		baseURL = strings.TrimRight(resource, "/") + "/v1/chat/completions"
	}
	request, err := http.NewRequestWithContext(incoming.Context(), http.MethodPost, baseURL, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	for key, value := range map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + accessToken, "User-Agent": "QwenCode/0.12.3 (linux; x64)", "X-DashScope-AuthType": "qwen-oauth", "X-DashScope-CacheControl": "enable", "X-DashScope-UserAgent": "QwenCode/0.12.3 (linux; x64)", "X-Stainless-Arch": "x64", "X-Stainless-Lang": "js", "X-Stainless-Os": "Linux", "X-Stainless-Package-Version": "5.11.0", "X-Stainless-Retry-Count": "1", "X-Stainless-Runtime": "node", "X-Stainless-Runtime-Version": "v18.19.1", "Connection": "keep-alive", "Accept-Language": "*", "Sec-Fetch-Mode": "cors"} {
		request.Header.Set(key, value)
	}
	if stream, _ := next["stream"].(bool); stream {
		request.Header.Set("Accept", "text/event-stream")
	} else {
		request.Header.Set("Accept", "application/json")
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

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}
