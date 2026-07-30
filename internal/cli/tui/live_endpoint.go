package tui

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type endpointRefreshMsg struct{}
type endpointActionDoneMsg struct {
	notice string
	err    error
}
type endpointDataMsg struct {
	status    tunnelStatus
	tailscale tailscaleStatus
	keys      []apiKey
	err       error
}

type endpointLiveModel struct {
	ui             *UI
	status         tunnelStatus
	tailscale      tailscaleStatus
	keys           []apiKey
	hasData        bool
	notice         string
	err            error
	loading        bool
	refreshing     bool
	actionRunning  bool
	controlRegions []tuiRegion
	cursor         int
}

func (ui *UI) liveEndpoint() error {
	EnableColors(ui.Out)
	model := endpointLiveModel{ui: ui, loading: true}
	return ui.runTea(&model)
}

func (model *endpointLiveModel) Init() tea.Cmd {
	model.loading = true
	return model.refreshCmd()
}

func (model *endpointLiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case endpointDataMsg:
		if message.err == nil {
			model.status, model.tailscale, model.keys, model.hasData = message.status, message.tailscale, message.keys, true
		}
		model.err, model.loading, model.refreshing = message.err, false, false
		return model, endpointRefresh()
	case tea.KeyMsg:
		key := message.String()
		if key == "q" || key == "esc" || key == "ctrl+c" {
			return model, tea.Quit
		}
		if model.loading || model.actionRunning {
			return model, nil
		}
		if model.err != nil && key == "r" {
			model.err = nil
			model.loading = true
			return model, model.refreshCmd()
		}
		if index, ok := endpointActionIndex(key); ok {
			model.cursor = index
			return model, model.runEndpointAction(index)
		}
		switch key {
		case "up", "k":
			model.cursor = cycleIndex(model.cursor, 8, -2)
		case "down", "j":
			model.cursor = cycleIndex(model.cursor, 8, 2)
		case "left", "h":
			model.cursor = cycleIndex(model.cursor, 8, -1)
		case "right", "l":
			model.cursor = cycleIndex(model.cursor, 8, 1)
		case "enter", " ":
			return model, model.runEndpointAction(model.cursor)
		case "t":
			return model, model.action(model.toggleTunnelIO)
		case "s":
			return model, model.action(model.toggleTailscale)
		case "c":
			return model, model.action(model.createKey)
		case "r":
			return model, model.action(model.renameKey)
		case "d":
			return model, model.action(model.deleteKey)
		case "v":
			return model, model.action(model.showKey)
		case "a":
			return model, model.action(model.toggleKey)
		}
	case tea.MouseMsg:
		if model.ui.viewClipped || model.loading || model.actionRunning {
			return model, nil
		}
		for index, region := range model.controlRegions {
			if region.contains(message.X, message.Y) {
				model.cursor = index
				if message.Action == tea.MouseActionPress && message.Button == tea.MouseButtonLeft {
					return model, model.runEndpointAction(index)
				}
				return model, nil
			}
		}
	case endpointRefreshMsg:
		if model.loading || model.refreshing {
			return model, endpointRefresh()
		}
		model.refreshing = true
		return model, model.refreshCmd()
	case endpointActionDoneMsg:
		model.actionRunning = false
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
			message.notice = ""
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.loading = true
			return model, model.refreshCmd()
		}
		return model, endpointRefresh()
	}
	return model, nil
}

func (model *endpointLiveModel) View() string {
	if model.loading {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("endpoint.title")) + "\n\n" + model.ui.loadingText(model.ui.t("common.loading")))
	}
	if model.err != nil && !model.hasData {
		return model.ui.errorView(model.ui.t("endpoint.title"), model.err)
	}
	endpointCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("endpoint.card")) + "\n" +
		endpointLine(model.ui, model.ui.t("endpoint.local"), mutedStyle.Render(apiEndpoint(model.ui.BaseURL))) + "\n" +
		endpointLine(model.ui, model.ui.t("endpoint.tunnel"), statusTextLocale(model.status.Tunnel.Enabled, model.ui.Locale)+"  "+mutedStyle.Render(apiEndpoint(model.status.Tunnel.PublicURL))) + "\n" +
		endpointLine(model.ui, model.ui.t("endpoint.tailscale"), statusTextLocale(model.status.Tailscale.Enabled, model.ui.Locale)+"  "+mutedStyle.Render(apiEndpoint(tailscaleState(model.status.Tailscale.TunnelURL, model.tailscale.Installed)))))
	keys := ""
	if len(model.keys) == 0 {
		keys = model.ui.t("keys.none")
	}
	visibleKeys := model.keys
	if len(visibleKeys) > 10 {
		visibleKeys = visibleKeys[:10]
	}
	for index, key := range visibleKeys {
		if keys != "" {
			keys += "\n"
		}
		keys += formatLiveKey(index+1, key, model.ui.Locale)
	}
	keysCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("keys.card")) + "\n" + keys + "\n\n" + mutedStyle.Render(model.ui.t("keys.selectShow")))
	controls := controlGrid(model) + "\n" + mutedStyle.Render(model.ui.t("keys.autoRefresh"))
	if model.notice != "" {
		controls += "\n" + successStyle.Render(model.notice)
	}
	if model.err != nil {
		controls += "\n" + errorStyle.Render(model.ui.t("common.error")+": "+model.ui.errorSummary(model.err))
	}
	controlsCard := model.ui.controlCard(model.ui.t("common.controls"), controls)
	view := model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("endpoint.title")) + "\n\n" + lipgloss.JoinVertical(lipgloss.Center, endpointCard, keysCard, controlsCard))
	model.controlRegions = endpointControlRegions(view, model.ui.t("common.controls"), len(endpointControlItems(model.ui)), model.ui.width, model.ui.compact())
	return view
}

func endpointControlRegions(view, title string, items, width int, compact bool) []tuiRegion {
	if items <= 0 {
		return nil
	}
	top := -1
	for index, line := range strings.Split(view, "\n") {
		if strings.Contains(line, title) {
			top = index + 2
			break
		}
	}
	if top < 0 {
		return nil
	}
	regions := make([]tuiRegion, items)
	columnWidth := max(1, width/2)
	for index := range regions {
		row, column := index, 0
		if !compact {
			row, column = index/2, index%2
		}
		regions[index] = tuiRegion{left: column * columnWidth, top: top + row, width: func() int {
			if compact {
				return max(1, width)
			}
			return columnWidth
		}(), height: 1}
	}
	return regions
}

func endpointLine(ui *UI, label, value string) string {
	if ui.compact() {
		labelWidth := min(len(label), max(1, ui.innerWidth()/3))
		label = ansi.Truncate(label, labelWidth, "…")
		return label + ": " + ansi.Truncate(value, max(1, ui.innerWidth()-lipgloss.Width(label)-2), "…")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, endpointLabelStyle.Render(label), value)
}

func controlGrid(model *endpointLiveModel) string {
	items := endpointControlItems(model.ui)
	return model.ui.controlColumnsSelected(model.cursor, items...)
}

func endpointControlItems(ui *UI) []string {
	return []string{
		"t " + ui.t("keys.tunnelToggle"),
		"s " + ui.t("keys.tailscaleToggle"),
		"c " + ui.t("keys.create"),
		"r " + ui.t("keys.rename"),
		"a " + ui.t("keys.toggle"),
		"d " + ui.t("keys.delete"),
		"v " + ui.t("keys.show"),
		"q " + ui.t("keys.back"),
	}
}

func endpointActionIndex(value string) (int, bool) {
	if len(value) != 1 || value[0] < '1' || value[0] > '8' {
		return 0, false
	}
	return int(value[0] - '1'), true
}

func (model *endpointLiveModel) runEndpointAction(index int) tea.Cmd {
	switch index {
	case 0:
		return model.action(model.toggleTunnelIO)
	case 1:
		return model.action(model.toggleTailscale)
	case 2:
		return model.action(model.createKey)
	case 3:
		return model.action(model.renameKey)
	case 4:
		return model.action(model.toggleKey)
	case 5:
		return model.action(model.deleteKey)
	case 6:
		return model.action(model.showKey)
	case 7:
		return tea.Quit
	default:
		return nil
	}
}

func (model *endpointLiveModel) refresh() {
	defer func() { model.loading = false }()
	var status tunnelStatus
	if err := model.ui.request(http.MethodGet, "/api/tunnel/status", nil, &status); err != nil {
		model.err = err
		return
	}
	var tailscale tailscaleStatus
	if err := model.ui.request(http.MethodGet, "/api/tunnel/tailscale-check", nil, &tailscale); err != nil {
		model.err = err
		return
	}
	var payload struct {
		Keys []apiKey `json:"keys"`
	}
	if err := model.ui.request(http.MethodGet, "/api/keys", nil, &payload); err != nil {
		model.err = err
		return
	}
	model.status, model.tailscale, model.keys, model.err = status, tailscale, payload.Keys, nil
}

func (model *endpointLiveModel) refreshCmd() tea.Cmd {
	ui := model.ui
	return func() tea.Msg {
		var status tunnelStatus
		if err := ui.request(http.MethodGet, "/api/tunnel/status", nil, &status); err != nil {
			return endpointDataMsg{err: err}
		}
		var tailscale tailscaleStatus
		if err := ui.request(http.MethodGet, "/api/tunnel/tailscale-check", nil, &tailscale); err != nil {
			return endpointDataMsg{err: err}
		}
		var payload struct {
			Keys []apiKey `json:"keys"`
		}
		if err := ui.request(http.MethodGet, "/api/keys", nil, &payload); err != nil {
			return endpointDataMsg{err: err}
		}
		return endpointDataMsg{status: status, tailscale: tailscale, keys: payload.Keys}
	}
}

func (model *endpointLiveModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
	model.actionRunning = true
	var notice string
	command := &endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		var err error
		notice, err = run(input, output)
		return err
	}}
	return tea.Exec(command, func(err error) tea.Msg {
		return endpointActionDoneMsg{notice: notice, err: err}
	})
}

func (model *endpointLiveModel) toggleTunnelIO(input io.Reader, output io.Writer) (string, error) {
	path := "/api/tunnel/enable"
	if model.status.Tunnel.Enabled {
		ok, err := model.ui.tuiConfirm(model.ui.t("form.disableTunnel"), input, output)
		if err != nil || !ok {
			return model.ui.t("keys.tunnelUnchanged"), err
		}
		path = "/api/tunnel/disable"
	}
	return model.ui.t("keys.tunnelUpdated"), model.ui.request(http.MethodPost, path, nil, nil)
}

func (model *endpointLiveModel) toggleTailscale(input io.Reader, output io.Writer) (string, error) {
	if !model.tailscale.Installed {
		return model.ui.t("common.notInstalled"), nil
	}
	path := "/api/tunnel/tailscale-enable"
	if model.status.Tailscale.Enabled {
		ok, err := model.ui.tuiConfirm(model.ui.t("form.disableTailscale"), input, output)
		if err != nil || !ok {
			return model.ui.t("keys.tailscaleUnchanged"), err
		}
		path = "/api/tunnel/tailscale-disable"
	}
	return model.ui.t("keys.tailscaleUpdated"), model.ui.request(http.MethodPost, path, nil, nil)
}

func (model *endpointLiveModel) createKey(input io.Reader, output io.Writer) (string, error) {
	_, err := model.ui.promptAPIKeyTUI(input, output)
	if err != nil {
		return "", err
	}
	return model.ui.t("keys.created"), nil
}

func (model *endpointLiveModel) renameKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.ui.tuiSelectKey(model.keys, input, output)
	if err != nil {
		return "", err
	}
	return model.ui.t("keys.rename"), model.ui.promptAPIKeyRenameTUI(&key, input, output)
}

func (model *endpointLiveModel) deleteKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.ui.tuiSelectKey(model.keys, input, output)
	if err != nil {
		return "", err
	}
	ok, err := model.ui.tuiConfirm(model.ui.t("form.delete"), input, output)
	if err != nil || !ok {
		return model.ui.t("keys.delete"), err
	}
	return model.ui.t("keys.delete"), model.ui.request(http.MethodDelete, "/api/keys/"+key.ID, nil, nil)
}

func (model *endpointLiveModel) showKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.ui.tuiSelectKey(model.keys, input, output)
	if err != nil {
		return "", err
	}
	ok, err := model.ui.tuiConfirm(model.ui.t("form.reveal"), input, output)
	if err != nil || !ok {
		return model.ui.t("keys.show"), err
	}
	var detail struct {
		Key apiKey `json:"key"`
	}
	if err := model.ui.request(http.MethodGet, "/api/keys/"+key.ID, nil, &detail); err != nil {
		return "", err
	}
	return fmt.Sprintf(model.ui.t("keys.value"), detail.Key.Key), nil
}

func (model *endpointLiveModel) toggleKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.ui.tuiSelectKey(model.keys, input, output)
	if err != nil {
		return "", err
	}
	return model.ui.t("keys.toggleUpdated"), model.ui.request(http.MethodPut, "/api/keys/"+key.ID, map[string]bool{"isActive": !key.IsActive}, nil)
}

type endpointExecCommand struct {
	input  io.Reader
	output io.Writer
	run    func(io.Reader, io.Writer) error
}

func (command *endpointExecCommand) Run() error                 { return command.run(command.input, command.output) }
func (command *endpointExecCommand) SetStdin(input io.Reader)   { command.input = input }
func (command *endpointExecCommand) SetStdout(output io.Writer) { command.output = output }
func (command *endpointExecCommand) SetStderr(io.Writer)        {}

func endpointRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return endpointRefreshMsg{} })
}

func formatLiveKey(index int, key apiKey, locale string) string {
	return strconv.Itoa(index) + ". " + key.Name + " [" + statusTextLocale(key.IsActive, locale) + "] " + maskSecret(key.Key)
}
