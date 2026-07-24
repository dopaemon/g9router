package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKiloFreeModelsCachedResponse(t *testing.T) {
	kiloModelsCache.Lock()
	kiloModelsCache.models = []map[string]any{{"id": "free-model", "isFree": true}}
	kiloModelsCache.at = time.Now()
	kiloModelsCache.Unlock()
	defer func() {
		kiloModelsCache.Lock()
		kiloModelsCache.models = nil
		kiloModelsCache.Unlock()
	}()
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/providers/kilo/free-models", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "free-model") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
