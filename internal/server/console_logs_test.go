package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsoleLogsAPI(t *testing.T) {
	consoleLogBuffer.Lock()
	consoleLogBuffer.lines = nil
	consoleLogBuffer.Unlock()
	addConsoleLog("test-line")
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/translator/console-logs", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "test-line") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLoggingSkipsLogViewerRequests(t *testing.T) {
	for _, path := range []string{"/api/usage/logs", "/api/translator/console-logs", "/api/translator/console-logs/stream"} {
		if shouldLogRequest(path) {
			t.Fatalf("log viewer request recorded: %s", path)
		}
	}
	if !shouldLogRequest("/api/providers") {
		t.Fatal("provider request was filtered")
	}
}
