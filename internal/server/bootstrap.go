package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var versionCache struct {
	sync.Mutex
	value string
	at    time.Time
}

func (s *Server) initAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Initialized"))
}

func (s *Server) versionAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	current := strings.TrimSpace(getenv("G9ROUTER_VERSION", "0.1.0"))
	latest := s.latestVersion(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"currentVersion": current, "latestVersion": latest, "hasUpdate": latest != "" && compareVersions(latest, current) > 0})
}

func (s *Server) latestVersion(parent context.Context) string {
	versionCache.Lock()
	defer versionCache.Unlock()
	if versionCache.value != "" && time.Since(versionCache.at) < time.Hour {
		return versionCache.value
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://registry.npmjs.org/9router/latest", nil)
	if err != nil {
		return ""
	}
	response, err := s.client.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ""
	}
	var payload struct {
		Version string `json:"version"`
	}
	if json.NewDecoder(response.Body).Decode(&payload) != nil {
		return ""
	}
	versionCache.value, versionCache.at = payload.Version, time.Now()
	return payload.Version
}

func compareVersions(left, right string) int {
	parse := func(value string) [3]int {
		var result [3]int
		parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
		for index := 0; index < len(parts) && index < 3; index++ {
			result[index], _ = strconv.Atoi(parts[index])
		}
		return result
	}
	a, b := parse(left), parse(right)
	for index := 0; index < 3; index++ {
		if a[index] > b[index] {
			return 1
		}
		if a[index] < b[index] {
			return -1
		}
	}
	return 0
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
