package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) providerTestModelsAPI(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if _, ok := s.store.Find(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Connection not found"})
		return
	}
	descriptor, ok := providers.Registry[id]
	if !ok {
		for providerID, candidate := range providers.Registry {
			if candidate.Alias == id {
				descriptor, ok = providers.Registry[providerID]
				break
			}
		}
	}
	if !ok || len(descriptor.Models) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No models configured for this provider"})
		return
	}
	results := make([]map[string]any, 0, len(descriptor.Models))
	for _, model := range descriptor.Models {
		kind := model.Kind
		if kind == "" {
			kind = "llm"
		}
		payload, _ := json.Marshal(map[string]string{"model": descriptor.Alias + "/" + model.ID, "kind": kind})
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
	writeJSON(w, http.StatusOK, map[string]any{"provider": id, "connectionId": id, "results": results})
}
