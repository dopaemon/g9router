package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunCLIToolsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli-tools/all-statuses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"claude":{"installed":true,"configured":false}}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := (&UI{BaseURL: server.URL, In: strings.NewReader(""), Out: &output, Client: server.Client()}).cliTools(bufio.NewReader(strings.NewReader("b\n"))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "claude") {
		t.Fatalf("output does not contain tool status: %s", output.String())
	}
}

func TestQuickSetupOpenCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/keys":
			_, _ = fmt.Fprint(w, `{"keys":[{"key":"sk-test"}]}`)
		case "/api/cli-tools/opencode-settings":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["apiKey"] != "sk-test" || body["activeModel"] != "cc/test" {
				t.Fatalf("payload = %#v", body)
			}
			_, _ = fmt.Fprint(w, `{"success":true}`)
		default:
			_, _ = fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, Out: &bytes.Buffer{}, Client: server.Client()}
	if err := ui.quickSetup(bufio.NewReader(strings.NewReader("opencode\ncc/test\n"))); err != nil {
		t.Fatal(err)
	}
}

func TestSelectClaudeModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli-tools/claude-settings" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["env"]["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "cc/opus-test" {
			t.Fatalf("payload = %#v", body)
		}
		_, _ = fmt.Fprint(w, `{"success":true}`)
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, Out: &bytes.Buffer{}, Client: server.Client()}
	if err := ui.selectClaudeModel(bufio.NewReader(strings.NewReader("opus\ncc/opus-test\n"))); err != nil {
		t.Fatal(err)
	}
}

func TestProvidersReadsConnectionsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/providers" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connections":[{"id":"openai","baseURL":"https://example.com","enabled":true}]}`)
	}))
	defer server.Close()

	var output strings.Builder
	ui := &UI{BaseURL: server.URL, Out: &output, Client: server.Client()}
	if err := ui.providers(bufio.NewReader(strings.NewReader("b\n"))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "openai (https://example.com) [enabled]") {
		t.Fatalf("output=%s", output.String())
	}
}

func TestProviderAndAuthSelections(t *testing.T) {
	ui := &UI{Out: &bytes.Buffer{}}
	item := provider{}
	if err := ui.selectProvider(bufio.NewReader(strings.NewReader("1\n")), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || item.Name == "" || item.APIType == "" {
		t.Fatalf("provider defaults = %#v", item)
	}
	mode, err := ui.selectAuthMode(bufio.NewReader(strings.NewReader("1\n")))
	if err != nil || mode != "oauth" {
		t.Fatalf("oauth mode=%q err=%v", mode, err)
	}
	mode, err = ui.selectAuthMode(bufio.NewReader(strings.NewReader("2\n")))
	if err != nil || mode != "apikey" {
		t.Fatalf("api key mode=%q err=%v", mode, err)
	}
}

func TestSecretsAreRedactedAndMutationsReportSuccess(t *testing.T) {
	if got := maskSecret("sk-1234567890"); got != "sk-1…7890" {
		t.Fatalf("maskSecret() = %q", got)
	}
	value := redactSecrets(map[string]any{"accessToken": "token-123456", "name": "demo"}).(map[string]any)
	if value["accessToken"] == "token-123456" || value["name"] != "demo" {
		t.Fatalf("redactSecrets() = %#v", value)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	var output bytes.Buffer
	ui := &UI{BaseURL: server.URL, Out: &output, Client: server.Client()}
	if err := ui.request(http.MethodPost, "/api/settings", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "POST /api/settings") {
		t.Fatalf("missing mutation feedback: %q", output.String())
	}
}

func TestRequestDoesNotExposeServerErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"token=secret-value"}`)
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, Out: &bytes.Buffer{}, Client: server.Client()}
	err := ui.request(http.MethodGet, "/api/providers", nil, nil)
	if err == nil || strings.Contains(err.Error(), "secret-value") || !strings.Contains(err.Error(), "502") {
		t.Fatalf("request error = %v", err)
	}
}
