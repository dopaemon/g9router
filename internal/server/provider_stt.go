package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) transcriptionProvider(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return false
	}
	defer file.Close()
	model := r.FormValue("model")
	for _, provider := range s.store.Resolve(model) {
		if provider.ID == "gemini" {
			if credential, ok := s.oauth.Get(provider.OAuthID); ok {
				provider.APIKey = credential.AccessToken
			}
			if s.geminiSTT(w, r, provider.APIKey, model, file, header, r.FormValue("language"), r.FormValue("prompt")) {
				return true
			}
			return false
		}
	}
	return false
}

func (s *Server) geminiSTT(w http.ResponseWriter, r *http.Request, apiKey, model string, file multipart.File, header *multipart.FileHeader, language, prompt string) bool {
	if apiKey == "" || model == "" || file == nil {
		return false
	}
	audio, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil || len(audio) == 0 {
		return false
	}
	if prompt == "" {
		prompt = "Generate a transcript of the speech. Return only the transcribed text, no commentary."
	}
	if language != "" {
		prompt += " Language: " + language + "."
	}
	mimeType := "application/octet-stream"
	if header != nil && header.Header.Get("Content-Type") != "" {
		mimeType = header.Header.Get("Content-Type")
	}
	body, _ := json.Marshal(map[string]any{"contents": []map[string]any{{"parts": []map[string]any{{"text": prompt}, {"inline_data": map[string]string{"mime_type": mimeType, "data": base64.StdEncoding.EncodeToString(audio)}}}}}})
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
	data, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	text := ""
	if len(payload.Candidates) > 0 {
		for _, part := range payload.Candidates[0].Content.Parts {
			text += part.Text
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
	return true
}
