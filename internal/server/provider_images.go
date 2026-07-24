package server

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) providerImage(w http.ResponseWriter, r *http.Request, providerID, apiKey string, providerData map[string]any, input map[string]any) bool {
	if providerID != "gemini" || apiKey == "" {
		switch providerID {
		case "stability-ai":
			return s.stabilityImage(w, r, apiKey, input)
		case "huggingface":
			return s.huggingFaceImage(w, r, apiKey, input)
		case "cloudflare-ai":
			return s.cloudflareImage(w, r, apiKey, providerData, input)
		case "fal-ai":
			return s.falImage(w, r, apiKey, input)
		case "black-forest-labs":
			return s.bflImage(w, r, apiKey, input)
		case "runwayml":
			return s.runwayImage(w, r, apiKey, input)
		case "sdwebui":
			return s.sdWebUIImage(w, r, input)
		case "nanobanana":
			return s.nanoBananaImage(w, r, apiKey, input)
		case "antigravity":
			return s.antigravityImage(w, r, apiKey, providerData, input)
		case "openai", "minimax", "openrouter", "recraft", "vercel-ai-gateway", "xai":
			return s.openAICompatibleImage(w, r, providerID, apiKey, input)
		case "codex":
			return s.codexImage(w, r, apiKey, providerData, input)
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

func (s *Server) codexImage(w http.ResponseWriter, r *http.Request, accessToken string, providerData map[string]any, input map[string]any) bool {
	if accessToken == "" || stringValue(input["prompt"]) == "" {
		return false
	}
	model := strings.TrimSuffix(stringValue(input["model"]), "-image")
	content := []map[string]any{{"type": "input_text", "text": stringValue(input["prompt"])}}
	body := map[string]any{
		"model": model, "instructions": "", "input": []any{map[string]any{"type": "message", "role": "user", "content": content}},
		"tools":       []any{map[string]any{"type": "image_generation", "output_format": "png", "size": stringValue(input["size"])}},
		"tool_choice": "auto", "parallel_tool_calls": false, "stream": true, "store": false, "reasoning": nil,
	}
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://chatgpt.com/backend-api/codex", strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Accept", "text/event-stream, application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("originator", "codex_cli_rs")
	request.Header.Set("User-Agent", "codex_cli_rs/0.136.0")
	request.Header.Set("version", "0.136.0")
	if accountID := stringValue(providerData["chatgptAccountId"]); accountID != "" {
		request.Header.Set("chatgpt-account-id", accountID)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 64<<20))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event struct {
			Item struct {
				Type   string `json:"type"`
				Result string `json:"result"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) == nil && event.Item.Type == "image_generation_call" && event.Item.Result != "" {
			writeJSON(w, http.StatusOK, imageResult([]map[string]string{{"b64_json": event.Item.Result}}))
			return true
		}
	}
	return false
}

func (s *Server) openAICompatibleImage(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	if apiKey == "" {
		return false
	}
	endpoint := map[string]string{
		"openai":            "https://api.openai.com/v1/images/generations",
		"minimax":           "https://api.minimaxi.com/v1/images/generations",
		"openrouter":        "https://openrouter.ai/api/v1/images/generations",
		"recraft":           "https://external.api.recraft.ai/v1/images/generations",
		"vercel-ai-gateway": "https://ai-gateway.vercel.sh/v1/images/generations",
		"xai":               "https://api.x.ai/v1/images/generations",
	}[providerID]
	model := stringValue(input["model"])
	prompt := stringValue(input["prompt"])
	if endpoint == "" || model == "" || prompt == "" {
		return false
	}
	count := 1
	if value, ok := input["n"].(float64); ok && value > 0 {
		count = int(value)
	}
	body := map[string]any{"model": model, "prompt": prompt, "n": count, "size": nonEmpty(stringValue(input["size"]), "1024x1024")}
	for _, key := range []string{"quality", "style", "response_format"} {
		if value, ok := input[key]; ok && value != nil && value != "" {
			body[key] = value
		}
	}
	if providerID == "xai" {
		for key := range body {
			if key != "model" && key != "prompt" && key != "n" && key != "response_format" {
				delete(body, key)
			}
		}
	}
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if providerID == "openrouter" {
		request.Header.Set("HTTP-Referer", "https://endpoint-proxy.local")
		request.Header.Set("X-Title", "Endpoint Proxy")
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	writeJSON(w, response.StatusCode, payload)
	return true
}

func (s *Server) antigravityImage(w http.ResponseWriter, r *http.Request, apiKey string, providerData map[string]any, input map[string]any) bool {
	model := stringValue(input["model"])
	if model == "" || apiKey == "" {
		return false
	}
	cleanModel := model
	if index := strings.LastIndex(cleanModel, "-"); index > 0 && strings.Contains(cleanModel[index+1:], "x") {
		cleanModel = cleanModel[:index]
	}
	projectID := stringValue(providerData["projectId"])
	if projectID == "" {
		projectID = stringValue(providerData["projectID"])
	}
	if projectID == "" {
		return false
	}
	body := map[string]any{
		"project": projectID, "model": cleanModel, "userAgent": "antigravity", "requestType": "image_gen",
		"requestId": "agent/" + uuidToken(projectID+model),
		"request": map[string]any{
			"contents":         []map[string]any{{"role": "user", "parts": []map[string]string{{"text": stringValue(input["prompt"])}}}},
			"generationConfig": map[string]any{"temperature": 1.0, "topP": 0.95, "topK": 40, "maxOutputTokens": 8192, "imageConfig": map[string]string{"aspectRatio": imageAspectRatio(stringValue(input["size"]))}},
			"sessionId":        projectID,
		},
	}
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://cloudcode-pa.googleapis.com/v1internal:generateContent", strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("User-Agent", "antigravity")
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
		images = append(images, map[string]string{"b64_json": "", "revised_prompt": stringValue(input["prompt"])})
	}
	writeJSON(w, http.StatusOK, imageResult(images))
	return true
}

func uuidToken(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	hexValue := fmt.Sprintf("%x", sum[:16])
	return hexValue[:8] + "/" + hexValue[8:20] + "/" + hexValue[20:]
}

func (s *Server) sdWebUIImage(w http.ResponseWriter, r *http.Request, input map[string]any) bool {
	size := strings.SplitN(stringValue(input["size"]), "x", 2)
	width, height := 512, 512
	if len(size) == 2 {
		width, _ = strconv.Atoi(size[0])
		height, _ = strconv.Atoi(size[1])
	}
	body := map[string]any{"prompt": input["prompt"], "width": width, "height": height, "steps": 20, "batch_size": numberOr(input["n"], 1)}
	return s.postImageJSON(w, r, "http://localhost:7860/sdapi/v1/txt2img", "", body, func(data []byte) (any, bool) {
		var payload struct {
			Images []string `json:"images"`
		}
		if json.Unmarshal(data, &payload) != nil {
			return nil, false
		}
		images := make([]map[string]string, 0, len(payload.Images))
		for _, image := range payload.Images {
			images = append(images, map[string]string{"b64_json": image})
		}
		return imageResult(images), true
	})
}

func (s *Server) nanoBananaImage(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	body := map[string]any{"prompt": input["prompt"], "type": "TEXTTOIAMGE", "numImages": numberOr(input["n"], 1), "image_size": imageAspectRatio(stringValue(input["size"])), "callBackUrl": "https://localhost/callback"}
	var images []string
	if image := stringValue(input["image"]); image != "" {
		body["type"] = "IMAGETOIAMGE"
		images = append(images, image)
	}
	if values, ok := input["images"].([]any); ok {
		for _, value := range values {
			if image := stringValue(value); image != "" {
				images = append(images, image)
			}
		}
	}
	if len(images) > 0 {
		body["type"] = "IMAGETOIAMGE"
		body["imageUrls"] = images
	}
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.nanobananaapi.ai/api/v1/nanobanana/generate", strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var submitted struct {
		Code int `json:"code"`
		Data struct {
			TaskID string `json:"taskId"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &submitted) != nil || submitted.Code != 200 || submitted.Data.TaskID == "" {
		return false
	}
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-r.Context().Done():
			return false
		case <-time.After(1500 * time.Millisecond):
		}
		poll, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.nanobananaapi.ai/api/v1/nanobanana/record-info?taskId="+url.QueryEscape(submitted.Data.TaskID), nil)
		if apiKey != "" {
			poll.Header.Set("Authorization", "Bearer "+apiKey)
		}
		result, err := s.client.Do(poll)
		if err != nil {
			return false
		}
		pollData, _ := io.ReadAll(io.LimitReader(result.Body, 8<<20))
		result.Body.Close()
		var status struct {
			Data struct {
				SuccessFlag  int    `json:"successFlag"`
				ErrorMessage string `json:"errorMessage"`
				Response     struct {
					ResultImageURL string `json:"resultImageUrl"`
					OriginImageURL string `json:"originImageUrl"`
				} `json:"response"`
			} `json:"data"`
		}
		if json.Unmarshal(pollData, &status) != nil {
			return false
		}
		if status.Data.SuccessFlag == 1 {
			image := status.Data.Response.ResultImageURL
			if image == "" {
				image = status.Data.Response.OriginImageURL
			}
			if image == "" {
				return false
			}
			writeJSON(w, http.StatusOK, imageResult([]map[string]string{{"url": image, "revised_prompt": stringValue(input["prompt"])}}))
			return true
		}
		if status.Data.SuccessFlag == 2 || status.Data.SuccessFlag == 3 {
			return false
		}
	}
	return false
}

func (s *Server) falImage(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	model := stringValue(input["model"])
	body := map[string]any{"prompt": input["prompt"], "num_images": numberOr(input["n"], 1)}
	if size := stringValue(input["size"]); size != "" {
		body["image_size"] = imageAspectRatio(size)
	}
	if image := stringValue(input["image"]); image != "" {
		body["image_url"] = image
	}
	return s.asyncImage(w, r, "https://queue.fal.run/"+strings.TrimPrefix(model, "models/"), apiKey, "Key", body, func(payload map[string]any) (any, bool) {
		images := []map[string]string{}
		if values, ok := payload["images"].([]any); ok {
			for _, value := range values {
				if item, ok := value.(map[string]any); ok {
					if image := stringValue(item["url"]); image != "" {
						images = append(images, map[string]string{"url": image})
					}
				}
			}
		}
		if image := stringValue(payload["image"]); image != "" {
			images = append(images, map[string]string{"url": image})
		}
		return imageResult(images), len(images) > 0
	})
}

func (s *Server) bflImage(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	model := stringValue(input["model"])
	body := map[string]any{"prompt": input["prompt"]}
	if size := stringValue(input["size"]); size != "" {
		parts := strings.SplitN(size, "x", 2)
		if len(parts) == 2 {
			body["width"], _ = strconv.Atoi(parts[0])
			body["height"], _ = strconv.Atoi(parts[1])
		}
	}
	if image := stringValue(input["image"]); image != "" {
		body["image_prompt"] = image
	}
	return s.asyncImage(w, r, "https://api.bfl.ai/v1/"+strings.TrimPrefix(model, "models/"), apiKey, "x-key", body, func(payload map[string]any) (any, bool) {
		result, _ := payload["result"].(map[string]any)
		if image := stringValue(result["sample"]); image != "" {
			return imageResult([]map[string]string{{"url": image}}), true
		}
		return nil, false
	})
}

func (s *Server) runwayImage(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	model := stringValue(input["model"])
	path := "image_to_video"
	if strings.Contains(model, "image") {
		path = "text_to_image"
	}
	body := map[string]any{"promptText": input["prompt"], "model": model, "ratio": imageAspectRatio(stringValue(input["size"]))}
	if image := stringValue(input["image"]); image != "" {
		if path == "text_to_image" {
			body["referenceImages"] = []map[string]string{{"uri": image}}
		} else {
			body["promptImage"] = image
		}
	}
	if path == "image_to_video" {
		body["duration"] = 5
	}
	return s.asyncImage(w, r, "https://api.dev.runwayml.com/v1/"+path, apiKey, "Authorization", body, func(payload map[string]any) (any, bool) {
		outputs := []map[string]string{}
		if values, ok := payload["output"].([]any); ok {
			for _, value := range values {
				if image, ok := value.(string); ok && image != "" {
					outputs = append(outputs, map[string]string{"url": image})
				}
			}
		}
		return imageResult(outputs), len(outputs) > 0
	})
}

func (s *Server) asyncImage(w http.ResponseWriter, r *http.Request, endpoint, apiKey, authHeader string, body any, normalize func(map[string]any) (any, bool)) bool {
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	if authHeader == "Authorization" {
		request.Header.Set(authHeader, "Bearer "+apiKey)
	} else {
		request.Header.Set(authHeader, apiKey)
	}
	if authHeader == "Authorization" && strings.Contains(endpoint, "runway") {
		request.Header.Set("X-Runway-Version", "2024-11-06")
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var submitted map[string]any
	if json.Unmarshal(data, &submitted) != nil {
		return false
	}
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-r.Context().Done():
			return false
		case <-time.After(1500 * time.Millisecond):
		}
		statusURL, responseURL := stringValue(submitted["status_url"]), stringValue(submitted["response_url"])
		if responseURL != "" && statusURL == "" {
			statusURL = responseURL
		}
		if statusURL == "" {
			base := strings.TrimSuffix(strings.TrimSuffix(endpoint, "/text_to_image"), "/image_to_video")
			statusURL = base + "/tasks/" + stringValue(submitted["id"])
		}
		poll, err := http.NewRequestWithContext(r.Context(), http.MethodGet, statusURL, nil)
		if err != nil {
			return false
		}
		if authHeader == "Authorization" {
			poll.Header.Set(authHeader, "Bearer "+apiKey)
		} else {
			poll.Header.Set(authHeader, apiKey)
		}
		poll.Header.Set("Accept", "application/json")
		result, err := s.client.Do(poll)
		if err != nil {
			return false
		}
		pollData, _ := io.ReadAll(io.LimitReader(result.Body, 8<<20))
		result.Body.Close()
		var status map[string]any
		if json.Unmarshal(pollData, &status) != nil {
			return false
		}
		state := stringValue(status["status"])
		if state == "FAILED" || state == "Failed" || state == "Error" || state == "CANCELLED" {
			return false
		}
		if state == "COMPLETED" || state == "SUCCEEDED" || state == "Ready" {
			if responseURL != "" && responseURL != statusURL {
				return s.fetchImageResult(w, r, responseURL, apiKey, authHeader, normalize)
			}
			value, ok := normalize(status)
			if !ok {
				return false
			}
			writeJSON(w, http.StatusOK, value)
			return true
		}
	}
	return false
}

func (s *Server) fetchImageResult(w http.ResponseWriter, r *http.Request, endpoint, apiKey, authHeader string, normalize func(map[string]any) (any, bool)) bool {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	if authHeader == "Authorization" {
		request.Header.Set(authHeader, "Bearer "+apiKey)
	} else {
		request.Header.Set(authHeader, apiKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	var payload map[string]any
	if response.StatusCode < 200 || response.StatusCode >= 300 || json.Unmarshal(data, &payload) != nil {
		return false
	}
	value, ok := normalize(payload)
	if !ok {
		return false
	}
	writeJSON(w, http.StatusOK, value)
	return true
}

func imageResult(images []map[string]string) map[string]any {
	return map[string]any{"created": time.Now().Unix(), "data": images}
}
func numberOr(value any, fallback int) int {
	if number, ok := value.(float64); ok && number > 0 {
		return int(number)
	}
	return fallback
}

func (s *Server) cloudflareImage(w http.ResponseWriter, r *http.Request, apiKey string, providerData map[string]any, input map[string]any) bool {
	accountID := stringValue(providerData["accountId"])
	model := stringValue(input["model"])
	if accountID == "" || model == "" || apiKey == "" {
		return false
	}
	if model == "@cf/black-forest-labs/flux-2-dev" || model == "@cf/black-forest-labs/flux-2-klein-4b" || model == "@cf/black-forest-labs/flux-2-klein-9b" {
		return s.cloudflareMultipartImage(w, r, apiKey, accountID, model, input)
	}
	body := map[string]any{"prompt": input["prompt"]}
	if size := stringValue(input["size"]); size != "" {
		parts := strings.SplitN(size, "x", 2)
		if len(parts) == 2 {
			if width, err := strconv.Atoi(parts[0]); err == nil {
				body["width"] = width
			}
			if height, err := strconv.Atoi(parts[1]); err == nil {
				body["height"] = height
			}
		}
	}
	for _, key := range []string{"negative_prompt", "guidance", "seed", "num_steps", "steps", "strength"} {
		if value, ok := input[key]; ok && value != nil && value != "" {
			body[key] = value
		}
	}
	endpoint := "https://api.cloudflare.com/client/v4/accounts/" + url.PathEscape(accountID) + "/ai/run/" + strings.TrimPrefix(model, "models/")
	return s.postImageJSON(w, r, endpoint, apiKey, body, func(data []byte) (any, bool) {
		var payload map[string]any
		if json.Unmarshal(data, &payload) != nil {
			return nil, false
		}
		result := payload["result"]
		if result == nil {
			result = payload
		}
		if resultMap, ok := result.(map[string]any); ok {
			if image, ok := resultMap["image"].(string); ok && image != "" {
				return map[string]any{"created": time.Now().Unix(), "data": []map[string]string{{"b64_json": image}}}, true
			}
		}
		return nil, false
	})
}

func (s *Server) cloudflareMultipartImage(w http.ResponseWriter, r *http.Request, apiKey, accountID, model string, input map[string]any) bool {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("prompt", stringValue(input["prompt"]))
	if size := stringValue(input["size"]); size != "" {
		parts := strings.SplitN(size, "x", 2)
		if len(parts) == 2 {
			_ = writer.WriteField("width", parts[0])
			_ = writer.WriteField("height", parts[1])
		}
	}
	for _, key := range []string{"negative_prompt", "guidance", "seed", "num_steps", "steps", "strength"} {
		if value, ok := input[key]; ok && value != nil && value != "" {
			_ = writer.WriteField(key, fmt.Sprint(value))
		}
	}
	_ = writer.Close()
	endpoint := "https://api.cloudflare.com/client/v4/accounts/" + url.PathEscape(accountID) + "/ai/run/" + modelPath(model)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, &body)
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "image/") {
		writeJSON(w, http.StatusOK, imageResult([]map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}))
		return true
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	result := payload["result"]
	if result == nil {
		result = payload
	}
	resultMap, _ := result.(map[string]any)
	image := stringValue(resultMap["image"])
	if image == "" {
		return false
	}
	writeJSON(w, http.StatusOK, imageResult([]map[string]string{{"b64_json": image}}))
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
	return s.postImageJSON(w, r, "https://api-inference.huggingface.co/models/"+modelPath(model), apiKey, body, func(data []byte) (any, bool) {
		if len(data) == 0 {
			return nil, false
		}
		return map[string]any{"created": time.Now().Unix(), "data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(data)}}}, true
	})
}

func modelPath(model string) string { return strings.Trim(strings.TrimPrefix(model, "models/"), "/") }

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
