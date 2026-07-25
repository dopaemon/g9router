package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) modelCatalogAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/models/")
	if path == r.URL.Path {
		path = strings.TrimPrefix(r.URL.Path, "/v1/models/")
	}
	path = strings.Trim(path, "/")
	if path == "info" {
		s.modelInfoAPI(w, r)
		return
	}
	kind := map[string]string{"image": "image", "tts": "tts", "stt": "stt", "embedding": "embedding", "image-to-text": "imageToText", "web": "webSearch"}[path]
	if kind == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"message": "Unknown model kind: " + path, "type": "invalid_request_error"}})
		return
	}
	data := []map[string]any{}
	for provider, descriptor := range providers.Registry {
		matchesService := containsString(descriptor.Services, kind)
		for _, model := range descriptor.Models {
			if model.Kind != "" {
				matchesService = model.Kind == kind || (kind == "webSearch" && model.Kind == "webFetch")
			}
			if !matchesService {
				continue
			}
			data = append(data, map[string]any{"id": descriptor.Alias + "/" + model.ID, "object": "model", "owned_by": provider, "name": model.Name, "type": model.Kind})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Server) modelInfoAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "Missing required query param: id", "type": "invalid_request_error"}})
		return
	}
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"message": "Model not found: " + id, "type": "not_found"}})
		return
	}
	for provider, descriptor := range providers.Registry {
		if descriptor.Alias != parts[0] && provider != parts[0] {
			continue
		}
		for _, model := range descriptor.Models {
			if model.ID == parts[1] {
				writeJSON(w, http.StatusOK, map[string]any{"id": id, "object": "model", "owned_by": provider, "name": model.Name, "kind": model.Kind, "services": descriptor.Services})
				return
			}
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"message": "Model not found: " + id, "type": "not_found"}})
}

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
