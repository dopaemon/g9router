package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) geminiEmbedding(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return false
	}
	var input struct {
		Model      string `json:"model"`
		Input      any    `json:"input"`
		Dimensions int    `json:"dimensions"`
	}
	if json.Unmarshal(data, &input) != nil || input.Model == "" {
		return false
	}
	for _, provider := range s.store.Resolve(input.Model) {
		if provider.ID != "gemini" {
			continue
		}
		if credential, ok := s.oauth.Get(provider.OAuthID); ok {
			provider.APIKey = credential.AccessToken
		}
		if provider.APIKey == "" {
			return false
		}
		model := "models/" + strings.TrimPrefix(input.Model, "models/")
		var body any
		operation := "embedContent"
		if values, ok := input.Input.([]any); ok {
			operation = "batchEmbedContents"
			requests := make([]map[string]any, 0, len(values))
			for _, value := range values {
				item := map[string]any{"model": model, "content": map[string]any{"parts": []map[string]string{{"text": stringValue(value)}}}}
				if input.Dimensions > 0 {
					item["outputDimensionality"] = input.Dimensions
				}
				requests = append(requests, item)
			}
			body = map[string]any{"requests": requests}
		} else {
			item := map[string]any{"model": model, "content": map[string]any{"parts": []map[string]string{{"text": stringValue(input.Input)}}}}
			if input.Dimensions > 0 {
				item["outputDimensionality"] = input.Dimensions
			}
			body = item
		}
		encoded, _ := json.Marshal(body)
		endpoint := "https://generativelanguage.googleapis.com/v1beta/" + model + ":" + operation + "?key=" + url.QueryEscape(provider.APIKey)
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(encoded)))
		if err != nil {
			return false
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := s.client.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		responseData, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return false
		}
		var payload struct {
			Embedding struct {
				Values []float64 `json:"values"`
			} `json:"embedding"`
			Embeddings []struct {
				Values []float64 `json:"values"`
			} `json:"embeddings"`
		}
		if json.Unmarshal(responseData, &payload) != nil {
			return false
		}
		embeddings := []map[string]any{}
		if len(payload.Embeddings) > 0 {
			for index, embedding := range payload.Embeddings {
				embeddings = append(embeddings, map[string]any{"object": "embedding", "index": index, "embedding": embedding.Values})
			}
		} else {
			embeddings = append(embeddings, map[string]any{"object": "embedding", "index": 0, "embedding": payload.Embedding.Values})
		}
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": embeddings, "model": input.Model, "usage": map[string]int{"prompt_tokens": 0, "total_tokens": 0}})
		return true
	}
	return false
}
