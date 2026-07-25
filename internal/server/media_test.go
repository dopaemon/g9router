package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestEmbeddingsProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatal(r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer upstream.Close()
	app := New(Options{Upstream: upstream.URL + "/v1", ProviderPath: os.TempDir() + "/media-test.json"})
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/embeddings", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.StatusCode)
	}
}

func TestMediaEndpointCORSPreflight(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json"})
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/embeddings", "/v1/images/generations", "/v1/audio/transcriptions"} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodOptions, path, nil))
		if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("path=%s status=%d headers=%v", path, response.Code, response.Header())
		}
	}
}
