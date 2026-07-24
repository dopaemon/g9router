package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) videoAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	path := strings.TrimPrefix(r.URL.Path, "/v1/videos/")
	path = strings.TrimPrefix(path, "/api/v1/videos/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing video request id"})
		return
	}
	action := ""
	if r.Method == http.MethodPost {
		if path != "generations" && path != "extensions" && path != "edits" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown video action: " + path})
			return
		}
		action = path
	} else if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider, ok := s.store.Find("xai")
	if ok && provider.OAuthID != "" {
		if credential, found := s.oauth.Get(provider.OAuthID); found {
			provider.APIKey = credential.AccessToken
		}
	}
	if !ok || strings.TrimSpace(provider.APIKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No credentials for provider: xai"})
		return
	}
	base := strings.TrimRight(provider.BaseURL, "/")
	base = strings.TrimSuffix(base, "/chat/completions")
	base = strings.TrimSuffix(base, "/v1") + "/v1"
	target := base + "/videos/" + path
	if action != "" {
		target = base + "/videos/" + action
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	var body io.Reader
	if r.Method == http.MethodPost {
		data, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
			return
		}
		body = strings.NewReader(string(data))
	}
	request, err := http.NewRequestWithContext(ctx, r.Method, target, body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+provider.APIKey)
	if r.Method == http.MethodPost {
		request.Header.Set("Content-Type", r.Header.Get("Content-Type"))
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			request.Header.Set("Idempotency-Key", key)
		}
	}
	response, err := s.client.Do(request)
	if err != nil {
		status := http.StatusBadGateway
		if ctx.Err() == context.DeadlineExceeded {
			status = http.StatusRequestTimeout
		}
		writeJSON(w, status, map[string]string{"error": fmt.Sprintf("[xai] video upstream fetch failed: %v", err)})
		return
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		if key == "Content-Length" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}
