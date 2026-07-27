package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"g9router/internal/i18n"
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
	if !ui.compact() {
		t.Fatal("narrow terminal should use compact layout")
	}
	if got := (&UI{width: 120}).innerStyle(100).GetWidth(); got > (&UI{width: 120}).innerWidth() {
		t.Fatalf("inner style width = %d, exceeds inner width", got)
	}
}

func TestMainMenuMouseItemUsesRenderedBounds(t *testing.T) {
	model := &mainMenuModel{items: []string{"one", "two"}, menuTop: 5, menuLeft: 4, menuWidth: 20}
	if got := model.mouseItem(4, 5); got != 0 {
		t.Fatalf("left edge item = %d, want 0", got)
	}
	if got := model.mouseItem(23, 6); got != 1 {
		t.Fatalf("right edge item = %d, want 1", got)
	}
	for _, point := range [][2]int{{3, 5}, {24, 5}, {4, 7}} {
		if got := model.mouseItem(point[0], point[1]); got != -1 {
			t.Fatalf("point %v = %d, want -1", point, got)
		}
	}
}

func TestProviderMouseRegionsUseRenderedBounds(t *testing.T) {
	model := &providerLiveModel{
		tabRegions: []tuiRegion{{left: 3, top: 5, width: 8, height: 1}, {left: 13, top: 5, width: 10, height: 1}},
		itemsTop:   11, itemsLeft: 6, itemsWidth: 20,
	}
	if got, ok := model.providerMouseTab(14, 5); !ok || got != oauthProviderTab {
		t.Fatalf("provider tab = %d, %v", got, ok)
	}
	if _, ok := model.providerMouseTab(11, 5); ok {
		t.Fatal("gap between tabs should not select a tab")
	}
	if got := model.providerMouseIndex(6, 12); got != 1 {
		t.Fatalf("provider row = %d, want 1", got)
	}
	if got := model.providerMouseIndex(5, 12); got != -1 {
		t.Fatalf("outside provider row = %d, want -1", got)
	}
}

func TestLanguageMouseRegionsUseRenderedCards(t *testing.T) {
	model := &languageModel{cardRegions: []tuiRegion{
		{left: 3, top: 5, width: 20, height: 5},
		{left: 3, top: 10, width: 20, height: 5},
	}}
	if !model.cardRegions[0].contains(4, 6) || !model.cardRegions[1].contains(4, 11) {
		t.Fatal("language card regions should contain their rendered rows")
	}
	if model.cardRegions[0].contains(24, 6) {
		t.Fatal("outside language card should not be clickable")
	}
}

func TestStatisticsTranslationsExist(t *testing.T) {
	for _, locale := range []string{"en", "vi"} {
		for _, key := range []string{"controls.period", "controls.tokenCursor", "stats.noUsage", "stats.noRequests"} {
			if got := i18n.T(locale, key); got == key || got == "" {
				t.Fatalf("missing %s translation for %s", key, locale)
			}
		}
	}
}

func TestSharedFormValidation(t *testing.T) {
	if err := validateProviderValues("", "http://example.test", "key"); err == nil {
		t.Fatal("empty provider ID should fail")
	}
	if err := validateProviderValues("id", "http://example.test", "key"); err != nil {
		t.Fatal(err)
	}
	if err := validateComboValues("combo", nil); err == nil {
		t.Fatal("empty combo models should fail")
	}
	if err := validateComboValues("combo", []string{"model"}); err != nil {
		t.Fatal(err)
	}
}

func TestErrorSummaryLocalizesHTTPFailures(t *testing.T) {
	ui := &UI{Locale: "en"}
	if got := ui.errorSummary(errors.New("HTTP 401 Unauthorized")); got != "Authentication failed. Check API key or endpoint." {
		t.Fatalf("auth summary = %q", got)
	}
	if got := ui.errorSummary(errors.New("dial timeout")); got != "Network error. Check the connection." {
		t.Fatalf("network summary = %q", got)
	}
}

func TestSettingsStatusSurvivesLocalizedLabels(t *testing.T) {
	if got := settingsActionItem(0, "Bật/tắt RTK", true, false); !strings.Contains(got, "ON") {
		t.Fatalf("localized settings status = %q", got)
	}
}

func TestCLIToolsCompactNavigationMovesOneRow(t *testing.T) {
	model := &cliToolsModel{ui: &UI{width: 42}, cursor: 2}
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if model.cursor != 3 {
		t.Fatalf("compact cursor = %d, want 3", model.cursor)
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

func TestLogsRowsKeepTenLineViewport(t *testing.T) {
	model := &logsModel{ui: &UI{Locale: "en"}, tab: 1, cursor: 11, httpLog: make([]string, 12)}
	for index := range model.httpLog {
		model.httpLog[index] = fmt.Sprintf("log-%d", index)
	}
	if got := strings.Count(model.rows(), "log-"); got != 10 {
		t.Fatalf("visible logs = %d", got)
	}
}

func TestLogsRowsFitNarrowTerminal(t *testing.T) {
	model := &logsModel{ui: &UI{width: 42}, apiLogs: []apiLogEntry{{Timestamp: "12:00:00", Status: "ok", Provider: "provider-name", Model: "a-very-long-model-name", Input: 1234, Output: 5678}}}
	if got := len([]rune(model.formatAPILog("12:00:00", "ok", model.apiLogs[0]))); got > model.ui.innerWidth()-2 {
		t.Fatalf("log row width = %d, limit = %d", got, model.ui.innerWidth()-2)
	}
}

func TestRedactLogText(t *testing.T) {
	got := redactLogText("Authorization: Bearer secret-token password=hidden")
	if strings.Contains(got, "secret-token") || strings.Contains(got, "hidden") {
		t.Fatalf("redacted log = %q", got)
	}
}

func TestLogsRefreshPreservesHealthySource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/usage/logs":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case "/api/translator/console-logs":
			_, _ = fmt.Fprint(w, `{"logs":["GET /api/test 200"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	model := &logsModel{
		ui:      &UI{BaseURL: server.URL, Client: server.Client(), Locale: "en"},
		apiLogs: []apiLogEntry{{Provider: "old"}},
		httpLog: []string{"old http"},
	}
	model.refresh()
	if model.apiErr == nil || model.httpErr != nil {
		t.Fatalf("errors = api %v, http %v", model.apiErr, model.httpErr)
	}
	if len(model.apiLogs) != 1 || model.apiLogs[0].Provider != "old" {
		t.Fatalf("api logs were discarded: %#v", model.apiLogs)
	}
	if len(model.httpLog) != 1 || model.httpLog[0] != "GET /api/test 200" {
		t.Fatalf("http logs = %#v", model.httpLog)
	}
}
