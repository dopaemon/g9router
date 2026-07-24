package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) proxyTestAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		ProxyURL string `json:"proxyUrl"`
		TestURL  string `json:"testUrl"`
		Timeout  int    `json:"timeoutMs"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.ProxyURL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "proxyUrl is required"})
		return
	}
	if input.TestURL == "" {
		input.TestURL = "https://httpbin.org/get"
	}
	if input.Timeout <= 0 || input.Timeout > 120000 {
		input.Timeout = 10000
	}
	proxyURL, err := url.Parse(input.ProxyURL)
	if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid proxyUrl"})
		return
	}
	target, err := url.Parse(input.TestURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid testUrl"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(input.Timeout)*time.Millisecond)
	defer cancel()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	client := &http.Client{Transport: transport}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		message := err.Error()
		if ctx.Err() == context.DeadlineExceeded {
			message = "Proxy test timed out"
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": message, "elapsedMs": time.Since(started).Milliseconds()})
		return
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	result := map[string]any{"ok": response.StatusCode >= 200 && response.StatusCode < 300, "status": response.StatusCode, "statusText": response.Status, "elapsedMs": time.Since(started).Milliseconds()}
	if !result["ok"].(bool) {
		result["error"] = "Proxy test failed"
		writeJSON(w, response.StatusCode, result)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
