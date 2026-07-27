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

	tea "github.com/charmbracelet/bubbletea"
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

func TestSecretsAreRedactedAndMutationsStayQuiet(t *testing.T) {
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
	if output.Len() != 0 {
		t.Fatalf("unexpected mutation output: %q", output.String())
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

func TestPromptAPIKeyRename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/keys/key-1" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["name"] != "renamed" {
			t.Fatalf("payload = %#v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, In: strings.NewReader("renamed\n1\n"), Out: &bytes.Buffer{}, Client: server.Client()}
	if err := ui.promptAPIKeyRename(&apiKey{ID: "key-1", Name: "old"}); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointLiveToggleTunnel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/tunnel/enable" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	model := endpointLiveModel{ui: &UI{BaseURL: server.URL, Out: &bytes.Buffer{}, Client: server.Client()}}
	if notice, err := model.toggleTunnelIO(strings.NewReader(""), &bytes.Buffer{}); err != nil || notice != "Tunnel updated" {
		t.Fatalf("toggle = %q, %v", notice, err)
	}
}

func TestAPIEndpoint(t *testing.T) {
	for input, want := range map[string]string{
		"http://127.0.0.1:20128":  "http://127.0.0.1:20128/v1",
		"https://example.test/v1": "https://example.test/v1",
		"not installed":           "not installed",
	} {
		if got := apiEndpoint(input); got != want {
			t.Fatalf("apiEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResponsiveCardWidths(t *testing.T) {
	ui := &UI{width: 42}
	if got := ui.cardWidth(); got > 38 {
		t.Fatalf("card width = %d, exceeds terminal content", got)
	}
	if got := ui.columnWidth(2) * 2; got > ui.innerWidth() {
		t.Fatalf("columns = %d, exceeds inner width %d", got, ui.innerWidth())
	}
}

func TestTUIFormMultiSelect(t *testing.T) {
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{{kind: tuiMultiSelect, options: []string{"a", "b"}, selected: []bool{false, false}}}}
	model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !model.fields[0].selected[0] || !model.fields[0].selected[1] {
		t.Fatalf("selected = %#v", model.fields[0].selected)
	}
}

func TestTUIFormInputKeepsNavigationLetters(t *testing.T) {
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{{kind: tuiInput}}}
	for _, value := range "Global" {
		model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}})
	}
	if got := model.fields[0].value; got != "Global" {
		t.Fatalf("input = %q", got)
	}
}

func TestTUIFormMultiSelectShowsTenCursorRows(t *testing.T) {
	options := make([]string, 12)
	for index := range options {
		options[index] = fmt.Sprintf("model-%d", index)
	}
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{{kind: tuiMultiSelect, options: options, selected: make([]bool, len(options))}}}
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	view := model.fieldValue(&model.fields[0])
	if strings.Count(view, "model-") != 10 || !strings.Contains(view, "> [ ] model-2") {
		t.Fatalf("multi-select view = %q", view)
	}
}
