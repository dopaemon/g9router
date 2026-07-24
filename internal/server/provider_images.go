package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) providerImage(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	if providerID != "gemini" || apiKey == "" {
		switch providerID {
		case "stability-ai":
			return s.stabilityImage(w, r, apiKey, input)
		case "huggingface":
			return s.huggingFaceImage(w, r, apiKey, input)
		default:
			return false
		}
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

func (s *Server) stabilityImage(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	model, _ := input["model"].(string)
	endpoint := "core"
	if strings.Contains(model, "ultra") {
		endpoint = "ultra"
	} else if strings.Contains(model, "sd3") {
		endpoint = "sd3"
	}
	body := map[string]any{"prompt": input["prompt"], "output_format": strings.ToLower(stringValue(input["output_format"]))}
	if body["output_format"] == "" {
		body["output_format"] = "png"
	}
	if size := stringValue(input["size"]); size != "" {
		body["aspect_ratio"] = imageAspectRatio(size)
	}
	if style := stringValue(input["style"]); style != "" {
		body["style_preset"] = style
	}
	if strings.Contains(model, "sd3") {
		body["model"] = model
	}
	return s.postImageJSON(w, r, "https://api.stability.ai/v2beta/stable-image/generate/"+endpoint, apiKey, body, func(data []byte) (any, bool) {
		var payload struct {
			Image string `json:"image"`
		}
		if json.Unmarshal(data, &payload) != nil || payload.Image == "" {
			return nil, false
		}
		return map[string]any{"created": time.Now().Unix(), "data": []map[string]string{{"b64_json": payload.Image}}}, true
	})
}

func (s *Server) huggingFaceImage(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	model, _ := input["model"].(string)
	body := map[string]string{"inputs": stringValue(input["prompt"])}
	return s.postImageJSON(w, r, "https://api-inference.huggingface.co/models/"+url.PathEscape(model), apiKey, body, func(data []byte) (any, bool) {
		if len(data) == 0 {
			return nil, false
		}
		return map[string]any{"created": time.Now().Unix(), "data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}}, true
	})
}

func (s *Server) postImageJSON(w http.ResponseWriter, r *http.Request, endpoint, apiKey string, body any, normalize func([]byte) (any, bool)) bool {
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	result, ok := normalize(data)
	if !ok {
		return false
	}
	writeJSON(w, http.StatusOK, result)
	return true
}

func imageAspectRatio(size string) string {
	parts := strings.SplitN(size, "x", 2)
	if len(parts) != 2 {
		return "1:1"
	}
	width, _ := strconv.Atoi(parts[0])
	height, _ := strconv.Atoi(parts[1])
	if width <= 0 || height <= 0 {
		return "1:1"
	}
	for _, ratio := range []struct {
		width, height int
		value         string
	}{{16, 9, "16:9"}, {9, 16, "9:16"}, {4, 3, "4:3"}, {3, 4, "3:4"}, {21, 9, "21:9"}, {9, 21, "9:21"}} {
		if width*ratio.height == height*ratio.width {
			return ratio.value
		}
	}
	return "1:1"
}
