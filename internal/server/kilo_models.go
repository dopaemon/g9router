package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var kiloModelsCache struct {
	sync.RWMutex
	models []map[string]any
	at     time.Time
}

func (s *Server) kiloFreeModelsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	kiloModelsCache.RLock()
	if len(kiloModelsCache.models) > 0 && time.Since(kiloModelsCache.at) < time.Hour {
		models := append([]map[string]any(nil), kiloModelsCache.models...)
		kiloModelsCache.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"models": models, "cached": true})
		return
	}
	kiloModelsCache.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.kilo.ai/api/gateway/models", nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"models": []any{}, "error": err.Error()})
		return
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeKiloFallback(w, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeKiloFallback(w, fmt.Errorf("Kilo API returned %d", response.StatusCode))
		return
	}
	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			IsFree        bool   `json:"isFree"`
			ContextLength int64  `json:"context_length"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		writeKiloFallback(w, err)
		return
	}
	models := []map[string]any{}
	for _, model := range payload.Data {
		if model.IsFree {
			models = append(models, map[string]any{"id": model.ID, "name": model.Name, "isFree": true, "context_length": model.ContextLength})
		}
	}
	kiloModelsCache.Lock()
	kiloModelsCache.models, kiloModelsCache.at = models, time.Now()
	kiloModelsCache.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"models": models, "cached": false})
}

func writeKiloFallback(w http.ResponseWriter, err error) {
	kiloModelsCache.RLock()
	models := append([]map[string]any(nil), kiloModelsCache.models...)
	kiloModelsCache.RUnlock()
	if len(models) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"models": models, "cached": true, "warning": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{"models": []any{}, "error": "Failed to fetch Kilo models: " + err.Error()})
}
