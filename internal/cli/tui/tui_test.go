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
	"os"
	"strings"
	"testing"

	"g9router/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestAccessibleMode(t *testing.T) {
	t.Setenv("G9ROUTER_ACCESSIBLE", "1")
	if !accessibleMode(os.Stdin) {
		t.Fatal("forced accessible mode is disabled")
	}
	if !accessibleMode(strings.NewReader("")) {
		t.Fatal("non-terminal input should use accessible mode")
	}
}

func TestLiveModeAvoidsBubbleTeaForAccessibleInput(t *testing.T) {
	t.Setenv("G9ROUTER_ACCESSIBLE", "1")
	ui := &UI{In: os.Stdin, forceHuh: true}
	if ui.liveMode() {
		t.Fatal("accessible input should not use live Bubble Tea screens")
	}
	ui.In = strings.NewReader("")
	if ui.liveMode() {
		t.Fatal("non-terminal input should not use live Bubble Tea screens")
	}
}

func TestPromptAPIKeyTUIUsesHuhWhenAccessible(t *testing.T) {
	t.Setenv("G9ROUTER_ACCESSIBLE", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/keys" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"id":"key-1","name":"new","key":"sk-test"}`)
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, In: strings.NewReader("new\n1\n"), Out: &bytes.Buffer{}, Client: server.Client()}
	created, err := ui.promptAPIKeyTUI(ui.In, ui.Out)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "key-1" {
		t.Fatalf("created key = %#v", created)
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
	if got := settingsActionItem(&UI{Locale: "en"}, 0, "Bật/tắt RTK", true, false); !strings.Contains(got, "ON") {
		t.Fatalf("localized settings status = %q", got)
	}
	if got := settingsActionItem(&UI{Locale: "vi"}, 0, "Bật/tắt RTK", true, false); !strings.Contains(got, "BẬT") {
		t.Fatalf("Vietnamese settings status = %q", got)
	}
}

func TestCLIToolsCompactNavigationMovesOneRow(t *testing.T) {
	model := &cliToolsModel{ui: &UI{width: 42}, cursor: 2}
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if model.cursor != 3 {
		t.Fatalf("compact cursor = %d, want 3", model.cursor)
	}
}

func TestEndpointLineFitsCompactWidth(t *testing.T) {
	ui := &UI{width: 42}
	if got := lipgloss.Width(endpointLine(ui, "Local", "https://example.test/v1/with/a/long/path")); got > ui.innerWidth() {
		t.Fatalf("endpoint line width = %d, limit = %d", got, ui.innerWidth())
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

func TestStatisticsPeriodsStackInCompactLayout(t *testing.T) {
	model := &statisticsModel{ui: &UI{width: 42, Locale: i18n.Vietnamese}, period: 2}
	view := model.View()
	if lipgloss.Height(view) < len(statisticsPeriods) {
		t.Fatalf("statistics view is too short: %d", lipgloss.Height(view))
	}
	if !strings.Contains(view, "7 ngày") {
		t.Fatalf("Vietnamese period label missing: %q", view)
	}
}

func TestLogsControlsListPauseAction(t *testing.T) {
	ui := &UI{Locale: i18n.English}
	controls := ui.actionHints(
		actionHint{"p", "logs.pause"},
		actionHint{"q", "logs.back"},
	)
	if !strings.Contains(controls, "p pause/resume") || !strings.Contains(controls, "q back") {
		t.Fatalf("controls = %q", controls)
	}
}

func TestMouseHintRequiresTerminalSize(t *testing.T) {
	ui := &UI{Locale: i18n.English}
	if ui.mouseHint() != "" {
		t.Fatal("mouse hint should stay hidden before terminal sizing")
	}
	ui.width = 80
	if !strings.Contains(ui.mouseHint(), "mouse") {
		t.Fatalf("mouse hint = %q", ui.mouseHint())
	}
}

func TestLiveViewsRespectShortTerminalHeight(t *testing.T) {
	const height = 12
	models := []struct {
		name string
		view func() string
	}{
		{"endpoint", func() string {
			return (&endpointLiveModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
		}},
		{"providers", func() string {
			return (&providerLiveModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
		}},
		{"combos", func() string {
			return (&comboLiveModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
		}},
		{"cli-tools", func() string {
			return (&cliToolsModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
		}},
		{"logs", func() string { return (&logsModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View() }},
		{"statistics", func() string {
			return (&statisticsModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
		}},
		{"settings", func() string {
			return (&settingsModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
		}},
		{"language", func() string {
			return (&languageModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
		}},
	}
	for _, model := range models {
		view := (&UI{height: height}).fitView(model.view())
		if got := lipgloss.Height(view); got > height {
			t.Errorf("%s view height = %d, want <= %d", model.name, got, height)
		}
	}
}

func TestProviderViewportKeepsLongListBounded(t *testing.T) {
	items := make([]provider, 20)
	for index := range items {
		items[index] = provider{ID: fmt.Sprintf("provider-%d", index), Name: fmt.Sprintf("Provider %d", index)}
	}
	model := &providerLiveModel{ui: &UI{width: 60, height: 16, Locale: i18n.Vietnamese}, custom: items, cursor: 15, tab: customProviderTab}
	view := model.cardContent()
	if model.itemsHeight > model.ui.viewportHeight(14, 10) {
		t.Fatalf("visible providers = %d", model.itemsHeight)
	}
	if model.cursor < model.itemsStart || model.cursor >= model.itemsStart+model.itemsHeight {
		t.Fatalf("cursor provider outside viewport: start=%d height=%d cursor=%d view=%q", model.itemsStart, model.itemsHeight, model.cursor, view)
	}
}

func TestLegacyTranslationsCoverVisibleGroups(t *testing.T) {
	ui := &UI{Locale: i18n.Vietnamese}
	for _, key := range []string{"oauth.title", "legacy.settings", "legacy.cliTools", "legacy.combos", "legacy.providers", "legacy.apiKeys", "legacy.invalidSelection"} {
		if value := ui.t(key); value == key || value == "" {
			t.Fatalf("missing translation for %s: %q", key, value)
		}
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

func TestLogDetailLabelsUseLocale(t *testing.T) {
	model := &logsModel{ui: &UI{Locale: "vi"}, cursor: 0, apiLogs: []apiLogEntry{{Timestamp: "now"}}}
	if detail := model.detailForCursor(); !strings.Contains(detail, "Thời gian:") || strings.Contains(detail, "Timestamp:") {
		t.Fatalf("localized detail = %q", detail)
	}
}

func TestProviderCompactTabsStack(t *testing.T) {
	model := &providerLiveModel{ui: &UI{width: 42, Locale: "en"}}
	view := model.View()
	if lipgloss.Height(view) == 0 || len(model.tabRegions) != 4 {
		t.Fatalf("provider compact view regions = %d", len(model.tabRegions))
	}
	if model.tabRegions[1].top != model.tabRegions[0].top+1 || model.itemsTop <= model.tabsTop+4 {
		t.Fatalf("provider compact geometry: tabs=%+v itemsTop=%d tabsTop=%d", model.tabRegions, model.itemsTop, model.tabsTop)
	}
}

func TestNavigationHelpers(t *testing.T) {
	if got := moveIndex(0, 3, -1); got != 0 || moveIndex(2, 3, 1) != 2 {
		t.Fatalf("clamped navigation failed")
	}
	if got := cycleIndex(0, 3, -1); got != 2 || cycleIndex(2, 3, 1) != 0 {
		t.Fatalf("cycled navigation failed")
	}
}

func TestHuhWidthFitsTerminal(t *testing.T) {
	if got := (&UI{width: 42}).huhWidth(); got > 38 {
		t.Fatalf("huh width = %d, exceeds terminal content", got)
	}
	if got := (&UI{}).huhWidth(); got != 72 {
		t.Fatalf("default huh width = %d, want 72", got)
	}
}

func TestLiveScreensRenderAtNarrowWidth(t *testing.T) {
	ui := &UI{width: 42, height: 24, Locale: "en"}
	views := []string{
		(&mainMenuModel{ui: ui, items: []string{"Endpoint", "Providers"}}).View(),
		(&endpointLiveModel{ui: ui}).View(),
		(&providerLiveModel{ui: ui}).View(),
		(&comboLiveModel{ui: ui}).View(),
		(&statisticsModel{ui: ui}).View(),
		(&cliToolsModel{ui: ui}).View(),
		(&settingsModel{ui: ui}).View(),
		(&languageModel{ui: ui}).View(),
		(&logsModel{ui: ui}).View(),
	}
	for index, view := range views {
		if strings.TrimSpace(view) == "" {
			t.Fatalf("screen %d rendered empty", index)
		}
	}
}

func TestLogsPauseRefresh(t *testing.T) {
	model := &logsModel{ui: &UI{Locale: "en"}}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !model.paused {
		t.Fatal("logs should pause")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if model.paused {
		t.Fatal("logs should resume")
	}
}

func TestLogsMouseUsesVisibleViewport(t *testing.T) {
	model := &logsModel{tab: 1, cursor: 11, httpLog: make([]string, 12), itemsRegion: tuiRegion{top: 10, left: 1, width: 20, height: 10}}
	if got := model.logIndexAtY(10); got != 2 {
		t.Fatalf("first visible log = %d, want 2", got)
	}
	if got := model.logIndexAtY(19); got != 11 {
		t.Fatalf("last visible log = %d, want 11", got)
	}
}

func TestLogsFollowNewestUntilUserMovesUp(t *testing.T) {
	logs := []string{"one", "two", "three"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/translator/console-logs" {
			_ = json.NewEncoder(w).Encode(map[string]any{"logs": logs})
			return
		}
		if r.URL.Path == "/api/usage/logs" {
			_ = json.NewEncoder(w).Encode([]apiLogEntry{})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	model := &logsModel{ui: &UI{BaseURL: server.URL, Client: server.Client()}, tab: 1, followTail: true}
	model.refresh()
	if model.cursor != 2 {
		t.Fatalf("initial tail cursor = %d, want 2", model.cursor)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if model.followTail || model.cursor != 1 {
		t.Fatalf("manual history cursor = %d, followTail=%v", model.cursor, model.followTail)
	}
	logs = append(logs, "four")
	model.refresh()
	if model.cursor != 1 {
		t.Fatalf("refresh moved history cursor to %d", model.cursor)
	}
}

func TestAPIKeyDetailUsesLocale(t *testing.T) {
	ui := &UI{Locale: "vi"}
	if got := fmt.Sprintf(ui.t("keys.value"), "sk-test"); got != "API key: sk-test" {
		t.Fatalf("API key detail = %q", got)
	}
}

func TestTUIFormRecoversAfterValidationError(t *testing.T) {
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{{label: "Combo Name", kind: tuiInput}}}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.err == nil {
		t.Fatal("empty input should fail validation")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.err != nil {
		t.Fatalf("valid input kept stale error: %v", model.err)
	}
}
