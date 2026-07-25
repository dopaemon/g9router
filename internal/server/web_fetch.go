package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func (s *Server) webFetchAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		URL           string `json:"url"`
		Provider      string `json:"provider"`
		Format        string `json:"format"`
		MaxCharacters int    `json:"max_characters"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}
	if input.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	target, err := url.Parse(input.URL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid URL"})
		return
	}
	if blockedFetchHost(target.Hostname()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "private or local URLs are not allowed"})
		return
	}
	provider, _ := s.store.Find(input.Provider)
	apiKey := provider.APIKey
	if credential, ok := s.oauth.Get(provider.OAuthID); ok {
		apiKey = credential.AccessToken
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	started := time.Now()
	var request *http.Request
	if input.Provider == "jina-reader" {
		request, err = http.NewRequestWithContext(ctx, http.MethodGet, "https://r.jina.ai/"+input.URL, nil)
		if apiKey != "" {
			request.Header.Set("Authorization", "Bearer "+apiKey)
		}
	} else if input.Provider == "firecrawl" {
		body, _ := json.Marshal(map[string]any{"url": input.URL, "formats": []string{nonEmpty(input.Format, "markdown")}})
		request, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://api.firecrawl.dev/v1/scrape", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			request.Header.Set("Authorization", "Bearer "+apiKey)
		}
	} else if input.Provider == "tavily" {
		body, _ := json.Marshal(map[string]any{"urls": []string{input.URL}, "extract_depth": "basic"})
		request, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/extract", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			request.Header.Set("Authorization", "Bearer "+apiKey)
		}
	} else if input.Provider == "exa" {
		body, _ := json.Marshal(map[string]any{"ids": []string{input.URL}, "text": true})
		request, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://api.exa.ai/contents", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			request.Header.Set("x-api-key", apiKey)
		}
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported provider: " + input.Provider})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, response.StatusCode, map[string]string{"error": fmt.Sprintf("%s: %s", input.Provider, strings.TrimSpace(string(data)))})
		return
	}
	text := string(data)
	if input.Provider == "firecrawl" {
		var payload struct {
			Data struct {
				Markdown string `json:"markdown"`
				Title    string `json:"title"`
				Metadata struct {
					Title string `json:"title"`
				} `json:"metadata"`
			} `json:"data"`
		}
		if json.Unmarshal(data, &payload) == nil {
			text = payload.Data.Markdown
			if text == "" {
				text = string(data)
			}
		}
	} else if input.Provider == "tavily" {
		var payload struct {
			Results []struct {
				RawContent string `json:"raw_content"`
			} `json:"results"`
		}
		if json.Unmarshal(data, &payload) == nil && len(payload.Results) > 0 {
			text = payload.Results[0].RawContent
		}
	} else if input.Provider == "exa" {
		var payload struct {
			Results []struct {
				Title string `json:"title"`
				Text  string `json:"text"`
			} `json:"results"`
		}
		if json.Unmarshal(data, &payload) == nil && len(payload.Results) > 0 {
			text = payload.Results[0].Text
		}
	}
	if input.MaxCharacters > 0 && len(text) > input.MaxCharacters {
		text = text[:input.MaxCharacters]
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": input.Provider, "url": input.URL, "title": nil, "content": map[string]any{"format": nonEmpty(input.Format, "markdown"), "text": text, "length": len(text)}, "metadata": map[string]any{"author": nil, "published_at": nil, "language": nil}, "usage": map[string]any{"fetch_cost_usd": nil}, "metrics": map[string]any{"response_time_ms": time.Since(started).Milliseconds()}})
}

func blockedFetchHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "metadata.google.internal" {
		return true
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() || address.IsMulticast()
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
