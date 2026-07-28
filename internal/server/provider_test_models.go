package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) providerTestModelsAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider, found := s.store.Find(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
		return
	}
	descriptor, ok := providers.Registry[provider.ID]
	if !ok {
		for providerID, candidate := range providers.Registry {
			if candidate.Alias == provider.ID {
				descriptor, ok = providers.Registry[providerID]
				break
			}
		}
	}
	models := []providers.Model{}
	if ok {
		models = descriptor.Models
	}
	if len(models) == 0 {
		models = s.discoverProviderModels(r, provider)
	}
	if len(models) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No models configured for this provider"})
		return
	}
	alias := provider.ID
	if ok && descriptor.Alias != "" {
		alias = descriptor.Alias
	}
	if r.Method == http.MethodGet {
		result := make([]map[string]any, 0, len(models))
		for _, model := range models {
			kind := model.Kind
			if kind == "" {
				kind = "llm"
			}
			result = append(result, map[string]any{"id": alias + "/" + model.ID, "name": model.Name, "kind": kind})
		}
		writeJSON(w, http.StatusOK, map[string]any{"provider": provider.ID, "models": result})
		return
	}
	results := make([]map[string]any, 0, len(models))
	for _, model := range models {
		kind := model.Kind
		if kind == "" {
			kind = "llm"
		}
		payload, _ := json.Marshal(map[string]string{"model": alias + "/" + model.ID, "kind": kind})
		probe := httptest.NewRequest(http.MethodPost, "/api/models/test", bytes.NewReader(payload))
		probe.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		s.modelTestAPI(recorder, probe)
		var result map[string]any
		if json.Unmarshal(recorder.Body.Bytes(), &result) != nil {
			result = map[string]any{"ok": false, "error": strings.TrimSpace(recorder.Body.String())}
		}
		result["modelId"], result["name"] = model.ID, model.Name
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": provider.ID, "connectionId": id, "results": results})
}

func (s *Server) discoverProviderModels(r *http.Request, provider providers.Provider) []providers.Model {
	base := strings.TrimRight(provider.BaseURL, "/")
	base = strings.TrimSuffix(base, "/chat/completions")
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil
	}
	if provider.APIType == "claude" || provider.APIType == "anthropic" {
		request.Header.Set("x-api-key", provider.APIKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	} else if provider.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil
	}
	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return nil
	}
	models := make([]providers.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := item.ID
		if id == "" {
			id = item.Name
		}
		if id != "" {
			models = append(models, providers.Model{ID: id, Name: valueOr(item.Name, id)})
		}
	}
	return models
}
