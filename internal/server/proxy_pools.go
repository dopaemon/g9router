package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"g9router/internal/proxypools"
)

var validProxyTypes = map[string]bool{"http": true, "vercel": true, "cloudflare": true, "deno": true}

func (s *Server) proxyPoolsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var active *bool
		if value := r.URL.Query().Get("isActive"); value == "true" || value == "false" {
			parsed := value == "true"
			active = &parsed
		}
		items := s.proxyPools.List(active)
		if r.URL.Query().Get("includeUsage") == "true" {
			items = s.withProxyBindingCounts(items)
		}
		writeJSON(w, http.StatusOK, map[string]any{"proxyPools": items})
	case http.MethodPost:
		var input struct {
			Name        string `json:"name"`
			ProxyURL    string `json:"proxyUrl"`
			NoProxy     string `json:"noProxy"`
			IsActive    *bool  `json:"isActive"`
			StrictProxy bool   `json:"strictProxy"`
			Type        string `json:"type"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		item, err := normalizeProxyPool(input.Name, input.ProxyURL, input.NoProxy, input.IsActive, input.StrictProxy, input.Type)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		created, err := s.proxyPools.Create(item)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"proxyPool": created})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func normalizeProxyPool(name, proxyURL, noProxy string, active *bool, strict bool, kind string) (proxypools.Pool, error) {
	name, proxyURL = strings.TrimSpace(name), strings.TrimSpace(proxyURL)
	if name == "" {
		return proxypools.Pool{}, fmt.Errorf("Name is required")
	}
	if proxyURL == "" {
		return proxypools.Pool{}, fmt.Errorf("Proxy URL is required")
	}
	if !validProxyTypes[kind] {
		kind = "http"
	}
	isActive := true
	if active != nil {
		isActive = *active
	}
	return proxypools.Pool{Name: name, ProxyURL: proxyURL, NoProxy: strings.TrimSpace(noProxy), IsActive: isActive, StrictProxy: strict, Type: kind}, nil
}

func (s *Server) proxyPoolResourceAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/proxy-pools/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "test" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		s.testProxyPoolAPI(w, r, parts[0])
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Proxy pool not found"})
		return
	}
	id := parts[0]
	item, ok := s.proxyPools.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Proxy pool not found"})
		return
	}
	item.BoundConnectionCount = s.proxyBindingCount(id)
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"proxyPool": item})
	case http.MethodPut:
		var updates map[string]any
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&updates) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := validateProxyUpdates(updates); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		updated, _, err := s.proxyPools.Update(id, updates)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"proxyPool": updated})
	case http.MethodDelete:
		if item.BoundConnectionCount > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "Proxy pool is currently in use", "boundConnectionCount": item.BoundConnectionCount})
			return
		}
		if err := s.proxyPools.Delete(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) withProxyBindingCounts(items []proxypools.Pool) []proxypools.Pool {
	for index := range items {
		items[index].BoundConnectionCount = s.proxyBindingCount(items[index].ID)
	}
	return items
}

func (s *Server) proxyBindingCount(id string) int {
	count := 0
	for _, provider := range s.store.List() {
		if provider.ProviderSpecificData == nil {
			continue
		}
		if bound, _ := provider.ProviderSpecificData["proxyPoolId"].(string); bound == id {
			count++
		}
	}
	return count
}

func validateProxyUpdates(updates map[string]any) error {
	if value, ok := updates["name"]; ok && (value == nil || strings.TrimSpace(fmt.Sprint(value)) == "") {
		return fmt.Errorf("Name is required")
	}
	if value, ok := updates["proxyUrl"]; ok && (value == nil || strings.TrimSpace(fmt.Sprint(value)) == "") {
		return fmt.Errorf("Proxy URL is required")
	}
	if value, ok := updates["type"].(string); ok && !validProxyTypes[value] {
		updates["type"] = "http"
	}
	return nil
}

func (s *Server) testProxyPoolAPI(w http.ResponseWriter, r *http.Request, id string) {
	item, ok := s.proxyPools.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Proxy pool not found"})
		return
	}
	started := time.Now()
	result := testProxyTarget(r.Context(), item)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	updates := map[string]any{"testStatus": "error", "lastTestedAt": now, "lastError": result.Error, "isActive": false}
	if result.OK {
		updates["testStatus"], updates["lastError"], updates["isActive"] = "active", nil, true
	}
	_, _, _ = s.proxyPools.Update(id, updates)
	response := map[string]any{"ok": result.OK, "status": result.Status, "statusText": result.StatusText, "error": result.Error, "elapsedMs": time.Since(started).Milliseconds(), "testedAt": now}
	writeJSON(w, http.StatusOK, response)
}

type proxyTestResult struct {
	OK         bool
	Status     int
	StatusText string
	Error      string
}

func testProxyTarget(parent context.Context, item proxypools.Pool) proxyTestResult {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	target := "https://httpbin.org/get"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if item.Type == "http" {
		proxyURL, err := url.Parse(item.ProxyURL)
		if err != nil {
			return proxyTestResult{Status: 500, Error: err.Error()}
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Transport: transport}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return proxyTestResult{Status: 500, Error: err.Error()}
	}
	if item.Type != "http" {
		request.Header.Set("x-relay-target", "https://httpbin.org")
		request.Header.Set("x-relay-path", "/get")
		target = item.ProxyURL
		request, err = http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return proxyTestResult{Status: 500, Error: err.Error()}
		}
		request.Header.Set("x-relay-target", "https://httpbin.org")
		request.Header.Set("x-relay-path", "/get")
	}
	response, err := client.Do(request)
	if err != nil {
		return proxyTestResult{Status: 500, Error: err.Error()}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return proxyTestResult{OK: response.StatusCode >= 200 && response.StatusCode < 300, Status: response.StatusCode, StatusText: response.Status}
}
