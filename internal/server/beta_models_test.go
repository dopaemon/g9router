package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBetaModelsUsesGeminiShape(t *testing.T) {
	app := New(Options{})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1beta/models", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var payload struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Models == nil {
		t.Fatal("models is nil")
	}
	if len(payload.Models) > 0 {
		if _, ok := payload.Models[0]["supportedGenerationMethods"]; !ok {
			t.Fatal("missing Gemini generation methods")
		}
	}
}
