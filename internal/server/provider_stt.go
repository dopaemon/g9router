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
		if credential, ok := s.oauth.Get(provider.OAuthID); ok {
			provider.APIKey = credential.AccessToken
		}
		switch provider.ID {
		case "gemini":
			return s.geminiSTT(w, r, provider.APIKey, model, file, header, r.FormValue("language"), r.FormValue("prompt"))
		case "deepgram":
			return s.deepgramSTT(w, r, provider.APIKey, model, file, header, r.FormValue("language"))
		case "huggingface":
			return s.huggingFaceSTT(w, r, provider.APIKey, strings.TrimPrefix(model, "huggingface/"), file, header)
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

func (s *Server) deepgramSTT(w http.ResponseWriter, r *http.Request, apiKey, model string, file multipart.File, header *multipart.FileHeader, language string) bool {
	if apiKey == "" {
		return false
	}
	audio, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		return false
	}
	query := url.Values{"model": {model}, "smart_format": {"true"}, "punctuate": {"true"}}
	if language != "" {
		query.Set("language", language)
	} else {
		query.Set("detect_language", "true")
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.deepgram.com/v1/listen?"+query.Encode(), strings.NewReader(string(audio)))
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", "Token "+apiKey)
	request.Header.Set("Content-Type", audioMIME(header))
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
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	text := ""
	if len(payload.Results.Channels) > 0 && len(payload.Results.Channels[0].Alternatives) > 0 {
		text = payload.Results.Channels[0].Alternatives[0].Transcript
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
	return true
}

func (s *Server) huggingFaceSTT(w http.ResponseWriter, r *http.Request, apiKey, model string, file multipart.File, header *multipart.FileHeader) bool {
	if strings.Contains(model, "..") || strings.Contains(model, "//") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model ID"})
		return true
	}
	audio, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api-inference.huggingface.co/models/"+model, strings.NewReader(string(audio)))
	if err != nil {
		return false
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	request.Header.Set("Content-Type", audioMIME(header))
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
		Text string `json:"text"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": payload.Text})
	return true
}

func audioMIME(header *multipart.FileHeader) string {
	if header != nil && header.Header.Get("Content-Type") != "" {
		return header.Header.Get("Content-Type")
	}
	return "application/octet-stream"
}
