package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

//go:embed pricing_defaults.json
var pricingDefaultsJSON []byte

func (s *Server) pricingAPI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/pricing/defaults" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, pricingDefaults())
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.mergedPricing())
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
		merged := s.mergedPricing()
		mergePricing(merged, pricing)
		if err := s.settings.Update(map[string]any{"pricing": merged}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update pricing"})
			return
		}
		writeJSON(w, http.StatusOK, merged)
	case http.MethodDelete:
		values := s.settings.Get()
		pricing, _ := values["pricing"].(map[string]any)
		if pricing == nil {
			pricing = map[string]any{}
		}
		provider, model := r.URL.Query().Get("provider"), r.URL.Query().Get("model")
		if provider == "" {
			pricing = map[string]any{}
		} else if model == "" {
			delete(pricing, provider)
		} else if models, ok := pricing[provider].(map[string]any); ok {
			delete(models, model)
		}
		if err := s.settings.Update(map[string]any{"pricing": pricing}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to reset pricing"})
			return
		}
		writeJSON(w, http.StatusOK, s.mergedPricing())
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func pricingDefaults() map[string]any {
	var values map[string]any
	if json.Unmarshal(pricingDefaultsJSON, &values) != nil || values == nil {
		return map[string]any{}
	}
	return values
}

func (s *Server) mergedPricing() map[string]any {
	merged := pricingDefaults()
	values := s.settings.Get()
	if pricing, ok := values["pricing"].(map[string]any); ok {
		mergePricing(merged, pricing)
	}
	return merged
}

func mergePricing(target, updates map[string]any) {
	for provider, rawModels := range updates {
		models, ok := rawModels.(map[string]any)
		if !ok {
			continue
		}
		current, _ := target[provider].(map[string]any)
		if current == nil {
			current = map[string]any{}
			target[provider] = current
		}
		for model, pricing := range models {
			current[model] = pricing
		}
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
