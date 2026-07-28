package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"g9router/internal/format"
	"g9router/internal/providers"
	"g9router/internal/translator"
)

func (s *Server) translatorAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Step int            `json:"step"`
		Body map[string]any `json:"body"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&input) != nil || input.Step == 0 || input.Body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "Step and body required"})
		return
	}
	clientBody := input.Body
	if nested, ok := input.Body["body"].(map[string]any); ok {
		clientBody = nested
	}
	model, _ := clientBody["model"].(string)
	provider := model
	if slash := strings.IndexByte(provider, '/'); slash >= 0 {
		provider = provider[:slash]
	}
	if provider == "" {
		provider = "openai"
	}
	source := format.Detect(clientBody)
	target := format.Target(provider)
	switch input.Step {
	case 1:
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]string{"provider": provider, "model": model, "sourceFormat": source, "targetFormat": target}})
	case 2:
		result := clientBody
		if source == format.Responses {
			result = translator.ResponsesToChat(clientBody)
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{"body": result}})
	case 3:
		providerName, _ := input.Body["provider"].(string)
		if providerName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "provider and model required"})
			return
		}
		descriptor, ok := providers.Lookup(providerName)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "unknown provider"})
			return
		}
		result := clientBody
		if source == format.Responses {
			result = translator.ResponsesToChat(clientBody)
		}
		headers := map[string]string{"Content-Type": "application/json"}
		for key, value := range descriptor.Headers {
			headers[key] = value
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "result": map[string]any{"url": descriptor.BaseURL, "headers": headers, "body": result}})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "Invalid step (1-3)"})
	}
}

func (s *Server) translatorSendAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Provider string         `json:"provider"`
		Model    string         `json:"model"`
		Body     map[string]any `json:"body"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&input) != nil || input.Provider == "" || input.Model == "" || input.Body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "provider, model, and body required"})
		return
	}
	body, err := json.Marshal(input.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
		return
	}
	for _, provider := range s.store.Resolve(input.Provider + "/" + input.Model) {
		if provider.ID != input.Provider && !strings.HasPrefix(provider.ID, input.Provider) {
			continue
		}
		provider, _ = s.credentialProvider(r.Context(), provider, false)
		if stream, _ := input.Body["stream"].(bool); stream {
			r.Header.Set("Accept", "text/event-stream")
		}
		if s.proxyWithExecutor(w, r, provider.BaseURL, "", body, provider.APIKey) {
			return
		}
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{"success": false, "error": "provider request failed"})
}
