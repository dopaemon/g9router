package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) antigravityAliasAPI(w http.ResponseWriter, r *http.Request) {
	settings := s.settings.Get()
	aliases, _ := settings["antigravityAliases"].(map[string]any)
	if aliases == nil {
		aliases = map[string]any{}
	}
	switch r.Method {
	case http.MethodGet:
		tool := strings.TrimSpace(r.URL.Query().Get("tool"))
		if value, ok := aliases[tool]; tool != "" && ok {
			writeJSON(w, http.StatusOK, map[string]any{"aliases": map[string]any{tool: value}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"aliases": aliases})
	case http.MethodPut:
		var input struct {
			Tool     string            `json:"tool"`
			Mappings map[string]string `json:"mappings"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Tool) == "" || input.Mappings == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool and mappings required"})
			return
		}
		clean := map[string]any{}
		for alias, model := range input.Mappings {
			if value := strings.TrimSpace(model); value != "" {
				clean[strings.TrimSpace(alias)] = value
			}
		}
		aliases[input.Tool] = clean
		if err := s.settings.Update(map[string]any{"antigravityAliases": aliases}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "aliases": clean})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
