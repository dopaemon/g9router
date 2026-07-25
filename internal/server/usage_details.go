package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"g9router/internal/usage"
)

func (s *Server) usageChartAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}
	if !map[string]bool{"today": true, "24h": true, "7d": true, "30d": true, "60d": true}[period] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid period"})
		return
	}
	logs := s.usage.Recent(1000)
	if period == "today" || period == "24h" {
		writeHourlyChart(w, logs, period)
		return
	}
	grouped := map[string]map[string]any{}
	for _, entry := range logs {
		when, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			continue
		}
		if !usageWithin(when, period) {
			continue
		}
		label := when.UTC().Format("2006-01-02")
		point := grouped[label]
		if point == nil {
			point = map[string]any{"label": label, "tokens": int64(0), "cost": float64(0)}
			grouped[label] = point
		}
		point["tokens"] = point["tokens"].(int64) + entry.Input + entry.Output
	}
	result := make([]map[string]any, 0, len(grouped))
	for _, point := range grouped {
		result = append(result, point)
	}
	writeSortedChart(w, result)
}

func writeHourlyChart(w http.ResponseWriter, logs []usage.Log, period string) {
	const bucketCount = 24
	bucketDuration := time.Hour
	start := time.Now()
	if period == "today" {
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	} else {
		start = start.Add(-bucketCount * bucketDuration)
	}
	buckets := make([]map[string]any, bucketCount)
	for index := range buckets {
		buckets[index] = map[string]any{"label": start.Add(time.Duration(index) * bucketDuration).Format("15:04"), "tokens": int64(0), "cost": float64(0)}
	}
	end := start.Add(bucketCount * bucketDuration)
	for _, entry := range logs {
		when, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil || when.Before(start) || !when.Before(end) {
			continue
		}
		index := int(when.Sub(start) / bucketDuration)
		buckets[index]["tokens"] = buckets[index]["tokens"].(int64) + entry.Input + entry.Output
	}
	writeJSON(w, http.StatusOK, buckets)
}

func usageWithin(when time.Time, period string) bool {
	now := time.Now()
	switch period {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return !when.Before(start)
	case "24h":
		return !when.Before(now.Add(-24 * time.Hour))
	case "7d":
		return !when.Before(now.AddDate(0, 0, -7))
	case "30d":
		return !when.Before(now.AddDate(0, 0, -30))
	case "60d":
		return !when.Before(now.AddDate(0, 0, -60))
	default:
		return false
	}
}

func writeSortedChart(w http.ResponseWriter, points []map[string]any) {
	for left := 0; left < len(points); left++ {
		for right := left + 1; right < len(points); right++ {
			if points[right]["label"].(string) < points[left]["label"].(string) {
				points[left], points[right] = points[right], points[left]
			}
		}
	}
	writeJSON(w, http.StatusOK, points)
}

func (s *Server) usageRequestDetailsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	page, pageSize := queryInt(r, "page", 1), queryInt(r, "pageSize", 20)
	if page < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Page must be >= 1"})
		return
	}
	if pageSize < 1 || pageSize > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "PageSize must be between 1 and 100"})
		return
	}
	provider, model, status := r.URL.Query().Get("provider"), r.URL.Query().Get("model"), r.URL.Query().Get("status")
	filtered := []usage.Log{}
	for _, entry := range s.usage.Recent(1000) {
		if provider != "" && entry.Provider != provider || model != "" && entry.Model != model || status != "" && entry.Status != status {
			continue
		}
		filtered = append(filtered, entry)
	}
	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(w, http.StatusOK, map[string]any{"details": filtered[start:end], "pagination": map[string]any{"page": page, "pageSize": pageSize, "total": total, "totalPages": (total + pageSize - 1) / pageSize}})
}

func queryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil {
		return fallback
	}
	return value
}
