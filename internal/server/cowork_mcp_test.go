package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoworkMCPToolsAPI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Mcp-Session-Id", "session-1")
		if strings.Contains(r.Header.Get("Accept"), "text/event-stream") && strings.Contains(readBody(t, r), "tools/list") {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"search\",\"description\":\"Find things\"}]}}\n\n")
			return
		}
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	}))
	defer upstream.Close()

	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	request := httptest.NewRequest(http.MethodPost, "/api/cli-tools/cowork-mcp-tools", strings.NewReader(`{"url":"`+upstream.URL+`/mcp"}`))
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"search"`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func readBody(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
