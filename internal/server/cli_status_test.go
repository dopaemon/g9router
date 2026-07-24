package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIToolsStatusIncludesAllPortedTools(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/cli-tools/all-statuses", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"cowork"`) || !strings.Contains(body, `"jcode"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, body)
	}
}
