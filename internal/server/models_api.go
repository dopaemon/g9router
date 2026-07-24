package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) modelsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		aliases := s.settings.ModelAliases()
		disabled := s.settings.DisabledModels()
		models := []map[string]any{}
		for provider, descriptor := range providers.Registry {
			providerAlias := descriptor.Alias
			if providerAlias == "" {
				providerAlias = provider
			}
			for _, model := range descriptor.Models {
				if containsString(disabled[providerAlias], model.ID) || containsString(disabled[provider], model.ID) {
					continue
				}
				fullModel := provider + "/" + model.ID
				routedModel := providerAlias + "/" + model.ID
				alias := aliases[fullModel]
				if alias == "" {
					alias = model.ID
				}
				models = append(models, map[string]any{"provider": provider, "model": model.ID, "name": model.Name, "kind": model.Kind, "fullModel": fullModel, "routedModel": routedModel, "alias": alias, "caps": map[string]any{"vision": false, "search": false, "reasoning": strings.Contains(strings.ToLower(model.ID), "reason"), "contextWindow": 0, "maxOutput": 0}})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": models})
	case http.MethodPut:
		var input struct {
			Model string `json:"model"`
			Alias string `json:"alias"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.Alias) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Model and alias required"})
			return
		}
		for model, alias := range s.settings.ModelAliases() {
			if model != input.Model && alias == input.Alias {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Alias already in use"})
				return
			}
		}
		if err := s.settings.SetModelAlias(input.Model, input.Alias); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update alias"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "model": input.Model, "alias": input.Alias})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
