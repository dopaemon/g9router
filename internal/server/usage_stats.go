package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) usageStatsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}
	if period != "all" && !map[string]bool{"today": true, "24h": true, "7d": true, "30d": true, "60d": true}[period] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid period"})
		return
	}
	writeJSON(w, http.StatusOK, s.usageStats(period))
}

func (s *Server) usageStats(period string) map[string]any {
	logs := s.usage.Recent(1000)
	stats := map[string]any{
		"totalRequests": 0, "totalPromptTokens": int64(0), "totalCompletionTokens": int64(0), "totalCachedTokens": int64(0), "totalCost": float64(0),
		"byProvider": map[string]int{}, "byModel": map[string]int{}, "byAccount": map[string]int{}, "byApiKey": map[string]int{}, "byEndpoint": map[string]int{},
		"last10Minutes": []map[string]any{}, "pending": map[string]any{}, "activeRequests": []any{}, "recentRequests": []any{}, "errorProvider": "",
	}
	byProvider, byModel := stats["byProvider"].(map[string]int), stats["byModel"].(map[string]int)
	recent := make([]map[string]any, 0, 20)
	for _, entry := range logs {
		when, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil || (period != "all" && !usageWithin(when, period)) {
			continue
		}
		stats["totalRequests"] = stats["totalRequests"].(int) + 1
		stats["totalPromptTokens"] = stats["totalPromptTokens"].(int64) + entry.Input
		stats["totalCompletionTokens"] = stats["totalCompletionTokens"].(int64) + entry.Output
		if entry.Status != "" && entry.Status != "ok" && entry.Provider != "" {
			stats["errorProvider"] = entry.Provider
		}
		if entry.Provider != "" {
			byProvider[entry.Provider]++
		}
		if entry.Model != "" {
			byModel[entry.Model]++
		}
		if len(recent) < 20 && (entry.Input != 0 || entry.Output != 0) {
			recent = append(recent, map[string]any{"timestamp": entry.Timestamp, "model": entry.Model, "provider": entry.Provider, "promptTokens": entry.Input, "completionTokens": entry.Output, "status": nonEmpty(entry.Status, "ok")})
		}
	}
	stats["recentRequests"] = recent
	return stats
}

func (s *Server) usageStreamAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	send := func() bool {
		data, err := json.Marshal(s.usageStats("all"))
		if err != nil {
			return false
		}
		if _, err = fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
			if !send() {
				return
			}
		}
	}
}
