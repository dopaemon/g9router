package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatCompletionProxiesRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer client-key" {
			t.Errorf("authorization was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"test"}`)
	}))
	defer upstream.Close()
	server := httptest.NewServer(New(Options{Upstream: upstream.URL + "/v1"}).Handler())
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"test","messages":[]}`))
	request.Header.Set("Authorization", "Bearer client-key")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
