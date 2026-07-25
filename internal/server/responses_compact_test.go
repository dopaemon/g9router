package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponsesCompactOptionsContract(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodOptions, "/v1/responses/compact", nil))
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
}
