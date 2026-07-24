package server

import (
	"net/http"
	"strings"
)

func (s *Server) usageResourceAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/usage/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "codex-reset-credits" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "provider quota reset is unavailable for this connection"})
		return
	}
	if len(parts) != 1 || parts[0] == "" || r.Method != http.MethodGet {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "usage connection not found"})
		return
	}
	connectionID := parts[0]
	providerName := ""
	for _, provider := range s.store.List() {
		if provider.ID == connectionID {
			providerName = provider.ID
			break
		}
		for _, account := range provider.Accounts {
			if account.ID == connectionID {
				providerName = provider.ID
				break
			}
		}
		if providerName != "" {
			break
		}
	}
	if providerName == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
		return
	}
	logs := s.usage.Recent(1000)
	var requests, input, output, errors int64
	for _, entry := range logs {
		if entry.Provider != providerName {
			continue
		}
		requests++
		input += entry.Input
		output += entry.Output
		if entry.Status != "" && entry.Status != "ok" {
			errors++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectionId": connectionID, "provider": providerName, "requests": requests, "errors": errors, "inputTokens": input, "outputTokens": output, "source": "g9router usage log"})
}
