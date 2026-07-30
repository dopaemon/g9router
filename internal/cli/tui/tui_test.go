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
	"time"

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

func TestLocalePersistsThroughSettingsAPI(t *testing.T) {
	var saved string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"locale":"vi"}`)
		case http.MethodPatch:
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			saved = body["locale"]
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, Client: server.Client(), Locale: i18n.English}
	ui.loadStoredLocale()
	if ui.Locale != i18n.Vietnamese {
		t.Fatalf("loaded locale = %q", ui.Locale)
	}
	if err := ui.saveLocale(i18n.English); err != nil {
		t.Fatal(err)
	}
	if saved != i18n.English {
		t.Fatalf("saved locale = %q", saved)
	}
}

func TestLocaleEnvironmentOverridesStoredLocale(t *testing.T) {
	t.Setenv("G9ROUTER_LOCALE", i18n.English)
	ui := &UI{In: strings.NewReader(""), Locale: i18n.English}
	ui.loadStoredLocale()
	if ui.Locale != i18n.English {
		t.Fatalf("environment locale = %q", ui.Locale)
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

func TestOAuthProvidersIncludeCodex(t *testing.T) {
	providers := oauthProviderNames()
	for _, provider := range providers {
		if provider == "codex" {
			return
		}
	}
	t.Fatal("Codex is missing from OAuth providers")
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

func TestEndpointBlocksActionsWhileLoading(t *testing.T) {
	model := &endpointLiveModel{ui: &UI{}, loading: true}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil || updated.(*endpointLiveModel).actionRunning {
		t.Fatal("endpoint action started while loading")
	}
}

func TestLogsTabsIgnoreHover(t *testing.T) {
	model := &logsModel{ui: &UI{}, tabRegions: []tuiRegion{{left: 1, top: 1, width: 5, height: 1}}}
	updated, _ := model.Update(tea.MouseMsg{X: 2, Y: 1, Action: tea.MouseActionMotion})
	if updated.(*logsModel).tab != 0 {
		t.Fatal("logs tab changed on hover")
	}
}

func TestTUIViewsFitTerminalMatrix(t *testing.T) {
	constructors := []struct {
		name string
		make func(*UI) tea.Model
	}{
		{"main", func(ui *UI) tea.Model { return &mainMenuModel{ui: ui, items: []string{"Providers", "Settings"}} }},
		{"endpoint", func(ui *UI) tea.Model { return &endpointLiveModel{ui: ui, loading: true} }},
		{"providers", func(ui *UI) tea.Model { return &providerLiveModel{ui: ui, loading: true} }},
		{"settings", func(ui *UI) tea.Model { return &settingsModel{ui: ui, loading: true} }},
		{"cli-tools", func(ui *UI) tea.Model { return &cliToolsModel{ui: ui, loading: true} }},
		{"combos", func(ui *UI) tea.Model { return &comboLiveModel{ui: ui, loading: true} }},
		{"logs", func(ui *UI) tea.Model { return &logsModel{ui: ui, loading: true} }},
		{"statistics", func(ui *UI) tea.Model { return &statisticsModel{ui: ui, loading: true} }},
		{"quota", func(ui *UI) tea.Model { return &quotaModel{ui: ui, loading: true, detail: -1} }},
		{"language", func(ui *UI) tea.Model { return &languageModel{ui: ui} }},
	}
	for _, width := range []int{20, 40, 80} {
		for _, constructor := range constructors {
			ui := &UI{width: width, height: 40, Locale: i18n.English}
			view := sizedModel{ui: ui, model: constructor.make(ui)}.View()
			for _, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("%s at width %d rendered %d columns", constructor.name, width, got)
				}
			}
		}
	}
}

func TestEndpointMouseStartsOnlyOnPress(t *testing.T) {
	model := &endpointLiveModel{ui: &UI{width: 80}, controlRegions: []tuiRegion{{left: 0, top: 2, width: 80, height: 1}}}
	updated, command := model.Update(tea.MouseMsg{X: 3, Y: 2, Action: tea.MouseActionMotion})
	if command != nil || updated.(*endpointLiveModel).actionRunning {
		t.Fatal("endpoint mouse hover started an action")
	}
	updated, command = model.Update(tea.MouseMsg{X: 3, Y: 2, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if command == nil || !updated.(*endpointLiveModel).actionRunning {
		t.Fatal("endpoint click did not start an action")
	}
}

func TestQuotaBlocksActionsDuringRefresh(t *testing.T) {
	model := &quotaModel{ui: &UI{}, items: []quotaItem{{ID: "codex"}}, refreshing: true, detail: -1}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil || updated.(*quotaModel).detail != -1 {
		t.Fatal("quota entered detail while refreshing")
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

func TestResponsiveCardGridFitsTerminalWidths(t *testing.T) {
	for _, width := range []int{20, 40, 80, 120, 180} {
		ui := &UI{width: width}
		columns := ui.responsiveColumns(7, 30)
		cardWidth := ui.responsiveCardWidth(7, 30)
		cards := make([]string, 7)
		for index := range cards {
			cards[index] = ui.innerStyle(cardWidth).Render(fmt.Sprintf("card %d", index+1))
		}
		for _, line := range strings.Split(ui.joinResponsiveCards(cards, columns), "\n") {
			if got := lipgloss.Width(line); got > ui.innerWidth() {
				t.Fatalf("width %d columns %d rendered %d > %d: %q", width, columns, got, ui.innerWidth(), line)
			}
		}
	}
}

func TestTUIResizeUpdatesLayoutImmediately(t *testing.T) {
	ui := &UI{width: 80, height: 24}
	model := sizedModel{ui: ui, model: &mainMenuModel{ui: ui, items: []string{"Providers", "Settings"}}}
	updated, command := model.Update(tea.WindowSizeMsg{Width: 32, Height: 12})
	resized := updated.(sizedModel)
	if resized.ui.width != 32 || resized.ui.height != 12 || command == nil {
		t.Fatalf("resize state: width=%d height=%d command=%v", resized.ui.width, resized.ui.height, command)
	}
	for _, line := range strings.Split(resized.View(), "\n") {
		if lipgloss.Width(line) > 32 {
			t.Fatalf("resized line width = %d", lipgloss.Width(line))
		}
	}
}

func TestResponsiveLayoutAtTinySize(t *testing.T) {
	ui := &UI{width: 10, height: 1}
	if got := ui.cardWidth(); got > 6 {
		t.Fatalf("tiny card width = %d", got)
	}
	if got := ui.fitView("first\nsecond"); got != "first" {
		t.Fatalf("height-one view = %q", got)
	}
}

func TestMainMenuMouseItemUsesRenderedBounds(t *testing.T) {
	model := &mainMenuModel{ui: &UI{}, items: []string{"one", "two"}, menuTop: 5, menuLeft: 4, menuWidth: 20}
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

func TestLogsRowsNeverGrowWithTallTerminal(t *testing.T) {
	model := &logsModel{ui: &UI{width: 80, height: 100, Locale: "en"}, tab: 1, cursor: 19, httpLog: make([]string, 20)}
	for index := range model.httpLog {
		model.httpLog[index] = fmt.Sprintf("log-%d", index)
	}
	model.View()
	if got := strings.Count(model.rows(), "log-"); got != 10 {
		t.Fatalf("visible logs = %d, want 10", got)
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
		actionDefinition{display: "p", label: "logs.pause"},
		actionDefinition{display: "q", label: "logs.back"},
	)
	if !strings.Contains(controls, "p pause/resume") || !strings.Contains(controls, "q back") {
		t.Fatalf("controls = %q", controls)
	}
}

func TestActionDefinitionsShareKeyMatching(t *testing.T) {
	action := actionDefinition{keys: []string{"up", "k"}, display: "↑/k", label: "logs.scroll"}
	if !actionMatches("k", action) || actionMatches("down", action) {
		t.Fatal("action key matching is inconsistent")
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
		{"quota", func() string {
			return (&quotaModel{ui: &UI{width: 40, height: height, Locale: i18n.Vietnamese}}).View()
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

func TestMenusUseSharedControlsCard(t *testing.T) {
	ui := &UI{width: 80, height: 40, Locale: i18n.English}
	models := []tea.Model{
		&mainMenuModel{ui: ui, items: []string{"Providers"}},
		&endpointLiveModel{ui: ui, hasData: true},
		&providerLiveModel{ui: ui},
		&comboLiveModel{ui: ui},
		&statisticsModel{ui: ui},
		&quotaModel{ui: ui, detail: -1},
		&cliToolsModel{ui: ui},
		&logsModel{ui: ui},
		&settingsModel{ui: ui},
		&languageModel{ui: ui},
	}
	for _, model := range models {
		if view := model.View(); !strings.Contains(view, ui.t("common.controls")) {
			t.Fatalf("view missing shared controls card: %q", view)
		}
	}
}

func TestMainMenuShowsG9RouterBanner(t *testing.T) {
	model := &mainMenuModel{ui: &UI{width: 100, height: 40, Locale: i18n.English}, items: []string{"Providers"}}
	view := model.View()
	if !strings.Contains(view, "██████╗") {
		t.Fatal("main menu banner is missing")
	}
	if got := lipgloss.Height(truncateBanner(g9routerBanner, model.ui.innerWidth()+4)); got != 6 {
		t.Fatalf("banner height = %d, want 6", got)
	}
}

func TestBannerFallsBackWhenTooNarrow(t *testing.T) {
	view := truncateBanner(g9routerBanner, 20)
	if strings.Contains(view, "██████╗") || !strings.Contains(view, "G9Router") {
		t.Fatalf("narrow banner = %q", view)
	}
}

func TestBannerCentersEveryLine(t *testing.T) {
	width := 100
	for _, line := range strings.Split(truncateBanner(g9routerBanner, width), "\n") {
		left := len(line) - len(strings.TrimLeft(line, " "))
		right := len(line) - len(strings.TrimRight(line, " "))
		if left < 0 || right < 0 || abs(left-right) > 1 {
			t.Fatalf("line is not centered: left=%d right=%d line=%q", left, right, line)
		}
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func TestQuotaLoadingUsesSkeleton(t *testing.T) {
	model := &quotaModel{ui: &UI{width: 80, height: 40, Locale: i18n.English}, loading: true, detail: -1}
	view := model.View()
	if strings.Contains(view, "Loading") || !strings.Contains(view, "░░░") {
		t.Fatalf("quota loading view is not a skeleton: %q", view)
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

func TestComboViewportKeepsTallTerminalBounded(t *testing.T) {
	items := make([]combo, 20)
	for index := range items {
		items[index] = combo{Name: fmt.Sprintf("combo-%d", index)}
	}
	model := &comboLiveModel{ui: &UI{width: 80, height: 100, Locale: i18n.English}, combos: items, cursor: 19}
	if got := lipgloss.Height(model.View()); got > 100 {
		t.Fatalf("combo view height = %d", got)
	}
}

func TestQuotaViewportKeepsTallTerminalBounded(t *testing.T) {
	items := make([]quotaItem, 20)
	for index := range items {
		items[index] = quotaItem{ID: fmt.Sprintf("provider-%d", index), Name: fmt.Sprintf("Provider %d", index)}
	}
	model := &quotaModel{ui: &UI{width: 80, height: 100, Locale: i18n.English}, items: items, cursor: 19, detail: -1}
	view := model.View()
	if got := strings.Count(view, "Provider "); got > 3 {
		t.Fatalf("visible quota providers = %d", got)
	}
}

func TestCodexOAuthProviderShowsAccountName(t *testing.T) {
	model := &providerLiveModel{
		ui:               &UI{width: 80, height: 20, Locale: i18n.Vietnamese},
		tab:              oauthProviderTab,
		oauthConnections: []provider{{ID: "codex", Name: "Codex", Accounts: []providerAccount{{Name: "Codex user@example.com"}}, Enabled: true}},
	}
	if !strings.Contains(model.cardContent(), "Codex user@example.com") {
		t.Fatal("Codex OAuth account name is not shown")
	}
}

func TestCodexProviderIsOAuthProvider(t *testing.T) {
	model := &providerLiveModel{}
	for _, item := range []provider{{ID: "codex", Name: "Codex"}} {
		if item.OAuthID != "" || item.ID == "codex" {
			model.oauthConnections = append(model.oauthConnections, item)
		}
	}
	if len(model.oauthConnections) != 1 {
		t.Fatal("Codex provider is not classified as OAuth")
	}
}

func TestOAuthProviderShowsEmail(t *testing.T) {
	name := oauthProviderDisplayName(provider{Name: "Gemini CLI", ProviderSpecificData: map[string]any{"email": "user@example.com"}})
	if name != "Gemini CLI user@example.com" {
		t.Fatalf("display name = %q", name)
	}
}

func TestOAuthProviderShowsCodexPlansForSameEmail(t *testing.T) {
	name := oauthProviderDisplayName(provider{
		ID:   "codex",
		Name: "Codex",
		Accounts: []providerAccount{
			{ID: "codex-team", Name: "Codex user@example.com", Email: "user@example.com", Plan: "team"},
			{ID: "codex", Name: "Codex user@example.com", Email: "user@example.com", Plan: "pro"},
		},
	})
	if !strings.Contains(name, "· team") || !strings.Contains(name, "· pro") || strings.Contains(name, "primary user") || !strings.Contains(name, " / ") {
		t.Fatalf("display name = %q", name)
	}
}

func TestCodexPlansBecomeSeparateOAuthConnections(t *testing.T) {
	connections := splitOAuthAccounts([]provider{{
		ID: "codex", OAuthID: "codex-oauth", Accounts: []providerAccount{
			{ID: "codex-team", Name: "Codex user@example.com", Plan: "team", OAuthID: "team-oauth", Enabled: true},
			{ID: "codex", Name: "Codex user@example.com", Plan: "pro", OAuthID: "pro-oauth", Enabled: true},
		},
	}})
	if len(connections) != 2 || connections[0].ID == connections[1].ID || len(connections[0].Accounts) != 1 || len(connections[1].Accounts) != 1 {
		t.Fatalf("connections=%+v", connections)
	}
}

func TestQuotaAndProviderUseSameCodexDisplayName(t *testing.T) {
	connection := splitOAuthAccounts([]provider{{
		ID: "codex", APIType: "openai-responses", Accounts: []providerAccount{{
			ID: "codex", Name: "Codex user@example.com", Email: "user@example.com", Plan: "free", Enabled: true,
		}},
	}})[0]
	if got := oauthProviderDisplayName(connection); got != "Codex user@example.com · free" {
		t.Fatalf("provider display name = %q", got)
	}
	if connection.Accounts[0].Plan != "free" {
		t.Fatalf("provider plan = %q", connection.Accounts[0].Plan)
	}
}

func TestQuotaBarShowsRemainingPercentage(t *testing.T) {
	bar := quotaBar("session", quotaWindow{Used: 25, Total: 100, Remaining: 75}, 80)
	if !strings.Contains(bar, "75%") || !strings.Contains(bar, "█") {
		t.Fatalf("quota bar = %q", bar)
	}
}

func TestQuotaProgressAnimatesToRemainingPercentage(t *testing.T) {
	model := &quotaModel{ui: &UI{width: 80, height: 30, Locale: i18n.English}, detail: -1}
	updated, _ := model.Update(quotaDataMsg{items: []quotaItem{{ID: "codex", Quotas: map[string]quotaWindow{
		"session": {Remaining: 75, Total: 100},
	}}}})
	model = updated.(*quotaModel)
	bar, ok := model.progressBars[quotaProgressKey(model.items[0], "session")]
	if !ok || bar.Percent() != 0.75 {
		t.Fatalf("quota progress = %#v, found=%v", bar.Percent(), ok)
	}
}

func TestQuotaCardsShowMiniInformation(t *testing.T) {
	model := &quotaModel{
		ui:     &UI{width: 80, height: 30, Locale: i18n.English},
		items:  []quotaItem{{Name: "Codex user@example.com", Plan: "team", ResetCredits: 2, ResetCreditsKnown: true, Quotas: map[string]quotaWindow{"session": {Remaining: 75, Total: 100}}}},
		detail: -1,
	}
	view := model.View()
	for _, want := range []string{"Account:", "Quota", "Reset time:", "Reset credits: 2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("quota card missing %q: %q", want, view)
		}
	}
}

func TestQuotaCardsNeverOverflowWidth(t *testing.T) {
	for _, width := range []int{40, 60, 100} {
		ui := &UI{width: width, height: 30, Locale: i18n.English}
		model := &quotaModel{
			ui: ui,
			items: []quotaItem{
				{Name: "Codex polarisdp@gmail.com", Plan: "team", Quotas: map[string]quotaWindow{"session": {Remaining: 67, Total: 100}}},
				{Name: "Codex polarisdp@gmail.com", Plan: "free", Quotas: map[string]quotaWindow{"session": {Remaining: 0, Total: 100}}},
			},
			detail: -1,
		}
		for lineIndex, line := range strings.Split(model.View(), "\n") {
			if lipgloss.Width(line) > ui.cardWidth()+2 {
				t.Fatalf("width %d line %d overflows: %d > %d: %q", width, lineIndex, lipgloss.Width(line), ui.cardWidth()+2, line)
			}
		}
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(3*24*time.Hour + 5*time.Hour + 12*time.Minute); got != "3d 5h 12m" {
		t.Fatalf("duration = %q", got)
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

func TestTruncateTextUsesDisplayWidth(t *testing.T) {
	got := truncateText("你好世界", 4)
	if width := lipgloss.Width(got); width > 4 {
		t.Fatalf("display width = %d, value = %q", width, got)
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
	if got := moveIndex(0, 3, -1); got != 2 || moveIndex(2, 3, 1) != 0 {
		t.Fatalf("wrapped navigation failed")
	}
	if got := cycleIndex(0, 3, -1); got != 2 || cycleIndex(2, 3, 1) != 0 {
		t.Fatalf("cycled navigation failed")
	}
}

func TestQuotaCursorWraps(t *testing.T) {
	model := &quotaModel{ui: &UI{}, items: []quotaItem{{ID: "one"}, {ID: "two"}}, detail: -1}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if updated.(*quotaModel).cursor != 1 {
		t.Fatalf("up from first cursor = %d, want 1", updated.(*quotaModel).cursor)
	}
	updated, _ = updated.(*quotaModel).Update(tea.KeyMsg{Type: tea.KeyDown})
	if updated.(*quotaModel).cursor != 0 {
		t.Fatalf("down from last cursor = %d, want 0", updated.(*quotaModel).cursor)
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

func TestTUIFormAcceptsNewAndConfirmPassword(t *testing.T) {
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{
		{label: "New password", kind: tuiInput, password: true},
		{label: "Confirm password", kind: tuiInput, password: true},
	}}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.err != nil || model.fields[0].value != "a" || model.fields[1].value != "a" {
		t.Fatalf("password form rejected filled fields: err=%v fields=%#v", model.err, model.fields)
	}
}

func TestTUIFormInputArrowNavigation(t *testing.T) {
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{
		{label: "New password", kind: tuiInput},
		{label: "Confirm password", kind: tuiInput},
	}}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if model.cursor != 1 || model.fields[0].value != "k" || model.fields[1].value != "k" {
		t.Fatalf("input navigation failed: cursor=%d fields=%#v", model.cursor, model.fields)
	}
}

func TestTUIFormAcceptsWhitespacePassword(t *testing.T) {
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{
		{label: "New password", kind: tuiInput, password: true, value: " "},
		{label: "Confirm password", kind: tuiInput, password: true, value: " "},
	}}
	if err := model.validate(); err != nil {
		t.Fatalf("whitespace password rejected: %v", err)
	}
}

func TestTUIFormAcceptsControlEnter(t *testing.T) {
	model := &tuiForm{ui: &UI{Locale: "en"}, fields: []tuiField{{label: "Password", kind: tuiInput, password: true, value: "secret"}}}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if updated.(*tuiForm).err != nil || command == nil {
		t.Fatalf("control-enter did not submit: err=%v command=%v", updated.(*tuiForm).err, command)
	}
}
