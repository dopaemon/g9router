package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) providerImage(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	if providerID != "gemini" || apiKey == "" {
		return false
	}
	prompt, _ := input["prompt"].(string)
	model, _ := input["model"].(string)
	if strings.TrimSpace(prompt) == "" || model == "" {
		return false
	}
	model = strings.TrimPrefix(model, "models/")
	body, _ := json.Marshal(map[string]any{
		"contents":         []map[string]any{{"parts": []map[string]string{{"text": prompt}}}},
		"generationConfig": map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}},
	})
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/" + url.PathEscape(model) + ":generateContent?key=" + url.QueryEscape(apiKey)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						Data string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	images := []map[string]string{}
	for _, candidate := range payload.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData.Data != "" {
				images = append(images, map[string]string{"b64_json": part.InlineData.Data})
			}
		}
	}
	if len(images) == 0 {
		images = append(images, map[string]string{"b64_json": "", "revised_prompt": prompt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": time.Now().Unix(), "data": images})
	return true
}
