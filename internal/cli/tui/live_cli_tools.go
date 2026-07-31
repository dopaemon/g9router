package tui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type cliToolsRefreshMsg struct{}
type cliToolsActionDoneMsg struct {
	notice string
	err    error
}
type cliToolsCodexModelsMsg struct {
	models []string
	err    error
}
type cliToolsCodexDoneMsg struct {
	detail string
	err    error
}
type cliToolsCodexOutputDoneMsg struct{ err error }
type cliToolsDataMsg struct {
	statuses map[string]cliToolStatus
	err      error
}

type cliToolStatus struct {
	Installed  bool `json:"installed"`
	Configured bool `json:"configured"`
	Available  bool `json:"available"`
}

type cliToolsModel struct {
	ui            *UI
	cursor        int
	statuses      map[string]cliToolStatus
	notice        string
	detail        string
	err           error
	codexStage    int
	codexMode     int
	codexCursor   int
	codexModels   []string
	codexLoading  bool
	loading       bool
	refreshing    bool
	actionRunning bool
}

var cliToolOrder = []string{"claude", "codex", "opencode", "droid", "openclaw", "hermes", "cowork", "copilot", "cline", "kilo", "deepseek-tui", "jcode", "grok-build"}

var cliToolLabels = map[string]string{
	"claude":       "Claude Code",
	"codex":        "OpenAI Codex CLI / App",
	"opencode":     "OpenCode",
	"droid":        "Factory Droid",
	"openclaw":     "Open Claw",
	"hermes":       "Hermes Agent",
	"cowork":       "Claude Cowork",
	"copilot":      "GitHub Copilot",
	"cline":        "Cline",
	"kilo":         "Kilo Code",
	"deepseek-tui": "DeepSeek TUI",
	"jcode":        "JCode",
	"grok-build":   "Grok Build",
}

var cliToolPaths = map[string]string{
	"claude": "/api/cli-tools/claude-settings", "codex": "/api/cli-tools/codex-settings", "opencode": "/api/cli-tools/opencode-settings", "droid": "/api/cli-tools/droid-settings", "openclaw": "/api/cli-tools/openclaw-settings", "hermes": "/api/cli-tools/hermes-settings", "cowork": "/api/cli-tools/cowork-settings", "copilot": "/api/cli-tools/copilot-settings", "cline": "/api/cli-tools/cline-settings", "kilo": "/api/cli-tools/kilo-settings", "deepseek-tui": "/api/cli-tools/deepseek-tui-settings", "jcode": "/api/cli-tools/jcode-settings", "grok-build": "/api/cli-tools/grok-build-settings",
}

func (ui *UI) liveCLITools() error {
	EnableColors(ui.Out)
	model := cliToolsModel{ui: ui, loading: true}
	return ui.runTea(&model)
}

func (model *cliToolsModel) Init() tea.Cmd {
	return model.refreshCmd()
}

func (model *cliToolsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if model.codexStage > 0 {
			return model.updateCodex(message)
		}
		if model.err != nil && message.String() == "r" {
			model.err = nil
			model.loading = true
			return model, model.refreshCmd()
		}
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= len(cliToolOrder) {
			if model.loading || model.actionRunning {
				return model, nil
			}
			model.cursor = index - 1
			return model, model.selectTool()
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			model.cursor = cycleIndex(model.cursor, len(cliToolOrder), -model.columns())
		case "down", "j":
			model.cursor = cycleIndex(model.cursor, len(cliToolOrder), model.columns())
		case "left", "h":
			model.cursor = cycleIndex(model.cursor, len(cliToolOrder), -1)
		case "right", "l":
			model.cursor = cycleIndex(model.cursor, len(cliToolOrder), 1)
		case "enter", " ", "s":
			if model.loading || model.actionRunning {
				return model, nil
			}
			return model, model.selectTool()
		case "r":
			if model.loading || model.actionRunning {
				return model, nil
			}
			return model, model.action(model.reset)
		}
	case cliToolsRefreshMsg:
		if model.actionRunning {
			return model, cliToolsRefresh()
		}
		if model.loading || model.refreshing {
			return model, cliToolsRefresh()
		}
		model.refreshing = true
		return model, model.refreshCmd()
	case cliToolsDataMsg:
		if message.err == nil {
			model.statuses = message.statuses
		}
		model.err, model.loading, model.refreshing = message.err, false, false
		return model, cliToolsRefresh()
	case cliToolsActionDoneMsg:
		model.actionRunning = false
		aborted := errors.Is(message.err, errUserAborted)
		if aborted {
			message.err = nil
			message.notice = ""
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.refreshing = true
			refresh := model.refreshCmd()
			if aborted {
				return model, tea.Batch(func() tea.Msg { return tea.ClearScreen() }, refresh)
			}
			return model, refresh
		}
		return model, nil
	case cliToolsCodexModelsMsg:
		model.codexLoading = false
		if message.err != nil {
			model.codexStage, model.err = 0, message.err
			return model, cliToolsRefresh()
		}
		model.codexModels, model.codexCursor = message.models, 0
		model.codexStage = 2
		return model, nil
	case cliToolsCodexDoneMsg:
		model.codexLoading = false
		model.codexStage = 0
		if message.err != nil {
			model.err = message.err
			return model, nil
		}
		return model, model.codexOutputCmd(message.detail)
	case cliToolsCodexOutputDoneMsg:
		model.detail = ""
		if message.err != nil {
			model.err = message.err
			return model, cliToolsRefresh()
		}
		model.refreshing = true
		return model, model.refreshCmd()
	}
	return model, nil
}

func (model *cliToolsModel) View() string {
	if model.loading || model.actionRunning {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.cliTools")) + "\n\n" + model.ui.loadingText(model.ui.t("common.loading")))
	}
	if model.err != nil && len(model.statuses) == 0 {
		return model.ui.errorView(model.ui.t("menu.cliTools"), model.err)
	}
	if model.codexStage > 0 {
		return model.codexView()
	}
	columns := model.columns()
	cards := make([]string, 0, (len(cliToolOrder)+columns-1)/columns)
	if columns == 1 {
		visible := max(1, min(5, model.ui.viewportHeight(8, 10)/2))
		start, end := viewportWindow(model.cursor, len(cliToolOrder), visible)
		for index := start; index < end; index++ {
			cards = append(cards, lipgloss.NewStyle().Width(model.ui.innerWidth()).Render(cliToolCard(model.ui, model.ui.innerWidth(), index, cliToolOrder[index], model.statuses[cliToolOrder[index]], index == model.cursor)))
		}
	} else {
		cardWidth := model.ui.responsiveCardWidth(len(cliToolOrder), 30)
		column := lipgloss.NewStyle().Width(cardWidth)
		rowCount := (len(cliToolOrder) + columns - 1) / columns
		visibleRows := max(1, min(3, model.ui.viewportHeight(8, 6)/2))
		startRow, endRow := viewportWindow(model.cursor/columns, rowCount, visibleRows)
		for row := startRow; row < endRow; row++ {
			rowCards := make([]string, 0, columns)
			for columnIndex := 0; columnIndex < columns; columnIndex++ {
				index := row*columns + columnIndex
				if index >= len(cliToolOrder) {
					break
				}
				rowCards = append(rowCards, column.Render(cliToolCard(model.ui, cardWidth, index, cliToolOrder[index], model.statuses[cliToolOrder[index]], index == model.cursor)))
			}
			cards = append(cards, lipgloss.JoinHorizontal(lipgloss.Top, rowCards...))
		}
	}
	controlsText := model.ui.controlColumns(
		model.ui.t("controls.toolsMove"), model.ui.t("controls.toolsSwitch"),
		model.ui.t("controls.toolsShow"), model.ui.t("controls.toolsReset"), model.ui.t("controls.toolsBack"),
	)
	if model.notice != "" {
		controlsText += "\n" + successStyle.Render(model.notice)
	}
	controls := model.ui.controlCard(model.ui.t("common.controls"), controlsText)
	content := lipgloss.JoinVertical(lipgloss.Left, cards...)
	if model.err != nil {
		content = errorStyle.Render(model.ui.t("common.error")+": "+model.ui.errorSummary(model.err)) + "\n\n" + content
	}
	if model.detail != "" {
		content += "\n\n" + model.ui.innerStyle().Render(cardTitleStyle.Render("New Codex configuration")+"\n"+model.detail)
	}
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.cliTools")) + "\n\n" + content + "\n\n" + controls)
}

func (model *cliToolsModel) selectTool() tea.Cmd {
	if cliToolOrder[model.cursor] != "codex" {
		return model.action(model.show)
	}
	model.detail = ""
	model.err = nil
	model.codexStage, model.codexMode, model.codexCursor = 1, 0, 0
	model.codexModels = nil
	return nil
}

func (model *cliToolsModel) updateCodex(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if model.codexLoading {
		return model, nil
	}
	switch message.String() {
	case "q", "esc", "ctrl+c":
		model.codexStage = 0
		return model, nil
	case "up", "k":
		model.codexCursor = cycleIndex(model.codexCursor, model.codexOptionCount(), -1)
	case "down", "j":
		model.codexCursor = cycleIndex(model.codexCursor, model.codexOptionCount(), 1)
	case "enter", " ":
		if model.codexStage == 1 {
			model.codexMode = model.codexCursor
			model.codexStage = 2
			model.codexLoading = true
			return model, model.codexModelsCmd()
		}
		model.codexLoading = true
		return model, model.codexApplyCmd()
	default:
		if len(message.Runes) == 1 && message.Runes[0] >= '1' && int(message.Runes[0]-'1') < model.codexOptionCount() {
			model.codexCursor = int(message.Runes[0] - '1')
		}
	}
	return model, nil
}

func (model *cliToolsModel) codexOptionCount() int {
	if model.codexStage == 1 {
		return 2
	}
	return len(model.codexModels)
}

func (model *cliToolsModel) codexView() string {
	options := []string{"Manual", "Auto"}
	label := "Mode"
	if model.codexStage == 2 {
		options, label = model.codexModels, "Model"
	}
	rows := make([]string, 0, len(options))
	for index, option := range options {
		marker := " "
		if index == model.codexCursor {
			marker = ">"
		}
		rows = append(rows, truncateText(fmt.Sprintf("%s %d  %s", marker, index+1, option), model.ui.innerWidth()-2))
	}
	if model.codexLoading {
		rows = append(rows, "", model.ui.loadingText(model.ui.t("common.loading")))
	}
	controls := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumns(
		"↑↓/jk move", "Enter select", "q back",
	))
	return model.ui.outerStyle().Render(cardTitleStyle.Render("Codex setup") + "\n\n" + cardTitleStyle.Render(label) + "\n" + strings.Join(rows, "\n") + "\n\n" + controls)
}

func (model *cliToolsModel) codexModelsCmd() tea.Cmd {
	ui := model.ui
	return func() tea.Msg {
		models, err := ui.codexModelOptions()
		return cliToolsCodexModelsMsg{models: models, err: err}
	}
}

func (model *cliToolsModel) codexApplyCmd() tea.Cmd {
	ui, selected, mode := model.ui, model.codexModels[model.codexCursor], model.codexMode
	return func() tea.Msg {
		if mode == 1 {
			return cliToolsCodexDoneMsg{err: ui.applyCodexSettings(selected)}
		}
		detail, err := ui.showCodexFiles(selected)
		return cliToolsCodexDoneMsg{detail: detail, err: err}
	}
}

func (model *cliToolsModel) codexOutputCmd(detail string) tea.Cmd {
	return tea.Exec(&endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		fmt.Fprintln(output, detail)
		fmt.Fprintln(output, "\nPress Enter to return to CLI Tools...")
		_, _ = bufio.NewReader(input).ReadString('\n')
		return nil
	}}, func(err error) tea.Msg { return cliToolsCodexOutputDoneMsg{err: err} })
}

func (model *cliToolsModel) columns() int {
	return model.ui.responsiveColumns(len(cliToolOrder), 30)
}

func cliToolCard(ui *UI, width, index int, id string, status cliToolStatus, selected bool) string {
	label := cliToolLabels[id]
	state := ui.t("status.notInstalled")
	color := "#94A3B8"
	if status.Installed && !status.Configured {
		state, color = ui.t("status.notConfigured"), "#FBBF24"
	}
	if status.Configured {
		state, color = ui.t("status.connected"), "#4ADE80"
	}
	title := fmt.Sprintf("%d  %s", index+1, label)
	if selected {
		title = focusStyle.Render(title)
	} else {
		title = cardTitleStyle.Render(title)
	}
	return innerCardStyle.Width(max(1, width-2)).Padding(0, 1).Render(title + "\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(state))
}

func (model *cliToolsModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
	model.actionRunning = true
	model.detail = ""
	var notice string
	return tea.Exec(&endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		var err error
		notice, err = run(input, output)
		return err
	}}, func(err error) tea.Msg { return cliToolsActionDoneMsg{notice: notice, err: err} })
}

func (model *cliToolsModel) show(input io.Reader, output io.Writer) (string, error) {
	if cliToolOrder[model.cursor] == "claude" {
		return "", model.ui.claudeSetup(input, output)
	}
	path := cliToolPaths[cliToolOrder[model.cursor]]
	if path == "" {
		return "settings unavailable", nil
	}
	oldIn, oldOut := model.ui.In, model.ui.Out
	model.ui.In, model.ui.Out = input, output
	err := model.ui.showJSON(bufio.NewReader(input), path)
	model.ui.In, model.ui.Out = oldIn, oldOut
	return "", err
}

func (ui *UI) claudeSetup(input io.Reader, output io.Writer) error {
	selected, err := ui.tuiSelect("Claude Code", "Context window", []string{"Default", "200K", "300K", "500K", "1M"}, input, output)
	if err != nil {
		return err
	}
	return ui.request(http.MethodPost, "/api/cli-tools/claude-settings", map[string]any{
		"env":              map[string]string{},
		"maxContextTokens": claudeContextTokens(selected),
	}, nil)
}

func claudeContextTokens(value string) string {
	return map[string]string{"200K": "198000", "300K": "298000", "500K": "498000", "1M": "998000"}[value]
}

func (ui *UI) codexModelOptions() ([]string, error) {
	var connections providersResponse
	if err := ui.request(http.MethodGet, "/api/providers", nil, &connections); err != nil {
		return nil, err
	}
	loggedIn := false
	for _, connection := range connections.Connections {
		if !strings.EqualFold(connection.ID, "codex") && !strings.EqualFold(connection.OAuthID, "codex") {
			continue
		}
		if connection.Enabled && len(connection.Accounts) == 0 {
			loggedIn = true
		}
		for _, account := range connection.Accounts {
			loggedIn = loggedIn || account.Enabled
		}
	}
	if !loggedIn {
		return nil, errors.New("no logged-in Codex account")
	}
	var payload struct {
		Models []struct {
			Provider    string `json:"provider"`
			RoutedModel string `json:"routedModel"`
			FullModel   string `json:"fullModel"`
			Model       string `json:"model"`
			Name        string `json:"name"`
		} `json:"models"`
	}
	if err := ui.request(http.MethodGet, "/api/models", nil, &payload); err != nil {
		return nil, err
	}
	options := make([]string, 0, len(payload.Models))
	for _, model := range payload.Models {
		if !strings.EqualFold(model.Provider, "codex") && !strings.HasPrefix(model.RoutedModel, "cx/") && !strings.HasPrefix(model.FullModel, "codex/") {
			continue
		}
		value := model.RoutedModel
		if value == "" {
			value = model.FullModel
		}
		if value == "" {
			value = model.Model
		}
		if value == "" {
			continue
		}
		if model.Name != "" && model.Name != value {
			value += " · " + model.Name
		}
		options = append(options, value)
	}
	if len(options) == 0 {
		return nil, errors.New("no models available")
	}
	return options, nil
}

func (ui *UI) showCodexFiles(model string) (string, error) {
	key, err := ui.firstAPIKey()
	if err != nil {
		return "", err
	}
	modelID := codexModelID(model)
	config := fmt.Sprintf("model = %q\nmodel_provider = \"9router\"\nmodel_reasoning_effort = \"medium\"\n\n[model_providers.9router]\nname = \"9Router\"\nbase_url = %q\nwire_api = \"responses\"\n\n[agents.subagent]\nmodel = %q\n", modelID, strings.TrimRight(ui.BaseURL, "/")+"/v1", modelID)
	auth, _ := json.MarshalIndent(map[string]string{"auth_mode": "apikey", "OPENAI_API_KEY": key}, "", "  ")
	return fmt.Sprintf("=== New ~/.codex/config.toml ===\n%s\n=== New ~/.codex/auth.json ===\n%s", config, auth), nil
}

func (ui *UI) applyCodexSettings(model string) error {
	key, err := ui.firstAPIKey()
	if err != nil {
		return err
	}
	return ui.request(http.MethodPost, "/api/cli-tools/codex-settings", map[string]any{
		"baseUrl": strings.TrimRight(ui.BaseURL, "/") + "/v1",
		"apiKey":  key,
		"model":   codexModelID(model),
	}, nil)
}

func (ui *UI) firstAPIKey() (string, error) {
	var payload struct {
		Keys []apiKey `json:"keys"`
	}
	if err := ui.request(http.MethodGet, "/api/keys", nil, &payload); err != nil {
		return "", err
	}
	if len(payload.Keys) == 0 {
		return "", errors.New("no API keys available")
	}
	key := payload.Keys[0].Key
	if strings.TrimSpace(key) == "" {
		var detail struct {
			Key apiKey `json:"key"`
		}
		if err := ui.request(http.MethodGet, "/api/keys/"+url.PathEscape(payload.Keys[0].ID), nil, &detail); err != nil {
			return "", err
		}
		key = detail.Key.Key
	}
	if strings.TrimSpace(key) == "" {
		return "", errors.New("selected API key is empty")
	}
	return key, nil
}

func codexModelID(value string) string { return strings.TrimSpace(strings.SplitN(value, " · ", 2)[0]) }

func (model *cliToolsModel) reset(input io.Reader, output io.Writer) (string, error) {
	id := cliToolOrder[model.cursor]
	ok, err := model.ui.tuiConfirm(fmt.Sprintf(model.ui.t("confirm.resetTool"), cliToolLabels[id]), input, output)
	if err != nil || !ok {
		return "", err
	}
	return model.ui.t("notice.settingsReset"), model.ui.request(http.MethodDelete, cliToolPaths[id], nil, nil)
}

func (model *cliToolsModel) refresh() {
	var statuses map[string]cliToolStatus
	if err := model.ui.request(http.MethodGet, "/api/cli-tools/all-statuses", nil, &statuses); err != nil {
		model.err = err
		return
	}
	model.statuses, model.err = statuses, nil
}

func (model *cliToolsModel) refreshCmd() tea.Cmd {
	ui := model.ui
	return func() tea.Msg {
		var statuses map[string]cliToolStatus
		if err := ui.request(http.MethodGet, "/api/cli-tools/all-statuses", nil, &statuses); err != nil {
			return cliToolsDataMsg{err: err}
		}
		return cliToolsDataMsg{statuses: statuses}
	}
}

func cliToolsRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return cliToolsRefreshMsg{} })
}
