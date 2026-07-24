package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (s *Server) pricingAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/pricing/defaults" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	switch r.Method {
	case http.MethodGet:
		values := s.settings.Get()
		pricing, _ := values["pricing"].(map[string]any)
		if pricing == nil {
			pricing = map[string]any{}
		}
		writeJSON(w, http.StatusOK, pricing)
	case http.MethodPatch:
		var pricing map[string]any
		if json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&pricing) != nil || pricing == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid pricing data format"})
			return
		}
		if err := validatePricing(pricing); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := s.settings.Update(map[string]any{"pricing": pricing}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update pricing"})
			return
		}
		writeJSON(w, http.StatusOK, pricing)
	case http.MethodDelete:
		if err := s.settings.Update(map[string]any{"pricing": map[string]any{}}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to reset pricing"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func validatePricing(pricing map[string]any) error {
	valid := map[string]bool{"input": true, "output": true, "cached": true, "reasoning": true, "cache_creation": true}
	for provider, rawModels := range pricing {
		models, ok := rawModels.(map[string]any)
		if !ok {
			return fmt.Errorf("Invalid pricing for provider: %s", provider)
		}
		for model, rawValues := range models {
			values, ok := rawValues.(map[string]any)
			if !ok {
				return fmt.Errorf("Invalid pricing for model: %s/%s", provider, model)
			}
			for field, rawValue := range values {
				if !valid[field] {
					return fmt.Errorf("Invalid pricing field: %s for %s/%s", field, provider, model)
				}
				value, ok := rawValue.(float64)
				if !ok || value < 0 {
					return fmt.Errorf("Invalid pricing value for %s in %s/%s: must be non-negative number", field, provider, model)
				}
			}
		}
	}
	return nil
}
