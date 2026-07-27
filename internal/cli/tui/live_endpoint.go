package tui

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"g9router/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type endpointRefreshMsg struct{}
type endpointActionDoneMsg struct {
	notice string
	err    error
}

type endpointLiveModel struct {
	ui        *UI
	status    tunnelStatus
	tailscale tailscaleStatus
	keys      []apiKey
	notice    string
	err       error
	cursor    int
}

func (ui *UI) liveEndpoint() error {
	EnableColors(ui.Out)
	model := endpointLiveModel{ui: ui}
	model.refresh()
	return ui.runTea(&model)
}

func (model *endpointLiveModel) Init() tea.Cmd {
	return endpointRefresh()
}

func (model *endpointLiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if index, ok := endpointActionIndex(message.String()); ok {
			model.cursor = index
			return model, model.runEndpointAction(index)
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			if model.cursor >= 2 {
				model.cursor -= 2
			}
		case "down", "j":
			if model.cursor+2 < 8 {
				model.cursor += 2
			}
		case "left", "h":
			if model.cursor%2 == 1 {
				model.cursor--
			}
		case "right", "l":
			if model.cursor%2 == 0 {
				model.cursor++
			}
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
	case endpointRefreshMsg:
		model.refresh()
		return model, endpointRefresh()
	case endpointActionDoneMsg:
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.refresh()
		}
		return model, endpointRefresh()
	}
	return model, nil
}

func (model *endpointLiveModel) View() string {
	if model.err != nil {
		return gradientText(model.ui.t("endpoint.title")) + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FB7185")).Render(model.ui.t("common.error")+": "+model.err.Error()) + "\n\n" + model.ui.t("common.back") + ".\n"
	}
	endpointCard := innerCardStyle.Render(cardTitleStyle.Render(model.ui.t("endpoint.card")) + "\n" +
		endpointLine(model.ui.t("endpoint.local"), mutedStyle.Render(apiEndpoint(model.ui.BaseURL))) + "\n" +
		endpointLine(model.ui.t("endpoint.tunnel"), statusTextLocale(model.status.Tunnel.Enabled, model.ui.Locale)+"  "+mutedStyle.Render(apiEndpoint(model.status.Tunnel.PublicURL))) + "\n" +
		endpointLine(model.ui.t("endpoint.tailscale"), statusTextLocale(model.status.Tailscale.Enabled, model.ui.Locale)+"  "+mutedStyle.Render(apiEndpoint(tailscaleState(model.status.Tailscale.TunnelURL, model.tailscale.Installed)))))
	keys := ""
	if len(model.keys) == 0 {
		keys = model.ui.t("keys.none")
	}
	for index, key := range model.keys {
		if keys != "" {
			keys += "\n"
		}
		keys += formatLiveKey(index+1, key, model.ui.Locale)
	}
	keysCard := innerCardStyle.Render(cardTitleStyle.Render(model.ui.t("keys.card")) + "\n" + keys + "\n\n" + mutedStyle.Render(model.ui.t("keys.selectShow")))
	controlsCard := innerCardStyle.Render(cardTitleStyle.Render(model.ui.t("keys.controls")) + "\n" + controlGrid(model))
	banner := lipgloss.NewStyle().Width(78).Align(lipgloss.Center).Render(gradientText(cliBanner))
	view := outerCardStyle.Render(banner + "\n\n" + lipgloss.JoinVertical(lipgloss.Center, endpointCard, keysCard, controlsCard))
	if model.notice != "" {
		view += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80")).Render(model.notice) + "\n"
	}
	view += "\n" + mutedStyle.Render(model.ui.t("keys.autoRefresh")) + "\n"
	return view
}

func endpointLine(label, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, endpointLabelStyle.Render(label), value)
}

func controlGrid(model *endpointLiveModel) string {
	items := []string{"t " + model.ui.t("keys.tunnelToggle"), "s " + model.ui.t("keys.tailscaleToggle"), "c " + model.ui.t("keys.create"), "r " + model.ui.t("keys.rename"), "a " + model.ui.t("keys.toggle"), "d " + model.ui.t("keys.delete"), "v " + model.ui.t("keys.show"), "q " + model.ui.t("keys.back")}
	column := lipgloss.NewStyle().Width(30)
	rows := make([]string, 0, 4)
	for index := 0; index < len(items); index += 2 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, column.Render(controlItem(items[index], index == model.cursor)), column.Render(controlItem(items[index+1], index+1 == model.cursor))))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func controlItem(label string, selected bool) string {
	style := controlsStyle
	if selected {
		style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1)
	}
	return style.Render(label)
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

func (model *endpointLiveModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
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
		ok, err := confirmHuh(input, output, "Disable Tunnel?", model.huhRun(input, output))
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
		ok, err := confirmHuh(input, output, "Disable Tailscale?", model.huhRun(input, output))
		if err != nil || !ok {
			return model.ui.t("keys.tailscaleUnchanged"), err
		}
		path = "/api/tunnel/tailscale-disable"
	}
	return model.ui.t("keys.tailscaleUpdated"), model.ui.request(http.MethodPost, path, nil, nil)
}

func (model *endpointLiveModel) createKey(input io.Reader, output io.Writer) (string, error) {
	_, err := promptAPIKeyIO(input, output, model.huhRun(input, output), model.ui.request, model.ui.Locale)
	if err != nil {
		return "", err
	}
	return model.ui.t("keys.created"), nil
}

func (model *endpointLiveModel) renameKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.selectKey(input, output)
	if err != nil {
		return "", err
	}
	return model.ui.t("keys.rename"), model.ui.promptAPIKeyRenameIO(&key, input, output, model.huhRun(input, output), model.ui.request, model.ui.Locale)
}

func (model *endpointLiveModel) deleteKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.selectKey(input, output)
	if err != nil {
		return "", err
	}
	ok, err := confirmHuh(input, output, model.ui.t("form.delete"), model.huhRun(input, output))
	if err != nil || !ok {
		return model.ui.t("keys.delete"), err
	}
	return model.ui.t("keys.delete"), model.ui.request(http.MethodDelete, "/api/keys/"+key.ID, nil, nil)
}

func (model *endpointLiveModel) showKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.selectKey(input, output)
	if err != nil {
		return "", err
	}
	ok, err := confirmHuh(input, output, model.ui.t("form.reveal"), model.huhRun(input, output))
	if err != nil || !ok {
		return model.ui.t("keys.show"), err
	}
	var detail struct {
		Key apiKey `json:"key"`
	}
	if err := model.ui.request(http.MethodGet, "/api/keys/"+key.ID, nil, &detail); err != nil {
		return "", err
	}
	return "API key: " + detail.Key.Key, nil
}

func (model *endpointLiveModel) toggleKey(input io.Reader, output io.Writer) (string, error) {
	key, err := model.selectKey(input, output)
	if err != nil {
		return "", err
	}
	return model.ui.t("keys.toggleUpdated"), model.ui.request(http.MethodPut, "/api/keys/"+key.ID, map[string]bool{"isActive": !key.IsActive}, nil)
}

func (model *endpointLiveModel) selectKey(input io.Reader, output io.Writer) (apiKey, error) {
	if len(model.keys) == 0 {
		return apiKey{}, errors.New("no API keys found")
	}
	options := make([]huh.Option[string], 0, len(model.keys))
	selected := model.keys[0].ID
	for _, key := range model.keys {
		options = append(options, huh.NewOption(key.Name, key.ID))
	}
	form := huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Title(model.ui.t("form.chooseKey")).Options(options...).Value(&selected)))
	if err := model.huhRun(input, output)(form); err != nil {
		return apiKey{}, err
	}
	for _, key := range model.keys {
		if key.ID == selected {
			return key, nil
		}
	}
	return apiKey{}, errors.New("invalid API key")
}

func (model *endpointLiveModel) huhRun(input io.Reader, output io.Writer) func(*huh.Form) error {
	return func(form *huh.Form) error { return model.ui.runHuhIO(form, input, output) }
}

func confirmHuh(input io.Reader, output io.Writer, title string, run func(*huh.Form) error) (bool, error) {
	value := false
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().Title(title).Value(&value)))
	if err := run(form); err != nil {
		return false, err
	}
	return value, nil
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

var outerCardStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7C3AED")).Padding(1, 2).Width(78)
var innerCardStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#A855F7")).Padding(1, 2).Width(66)
var cardTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F0ABFC"))
var endpointLabelStyle = lipgloss.NewStyle().Width(12).Foreground(lipgloss.Color("#CBD5E1"))
var controlsStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
var mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))

func statusText(enabled bool) string {
	return statusTextLocale(enabled, "en")
}

func statusTextLocale(enabled bool, locale string) string {
	color := "#FB7185"
	if enabled {
		color = "#4ADE80"
	}
	label := "common.off"
	if enabled {
		label = "common.on"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(i18n.T(locale, label))
}

func gradientText(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	result := ""
	for index, value := range runes {
		ratio := float64(index) / float64(len(runes)-1)
		if len(runes) == 1 {
			ratio = 0
		}
		red := int(34 + 217*ratio)
		green := int(211 - 131*ratio)
		blue := int(238 + 17*ratio)
		result += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", red, green, blue))).Render(string(value))
	}
	return result
}
