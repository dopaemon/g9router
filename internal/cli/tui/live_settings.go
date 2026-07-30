package tui

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

type settingsRefreshMsg struct{}
type settingsActionDoneMsg struct {
	notice string
	err    error
}
type settingsDataMsg struct {
	values, tunnel map[string]any
	err            error
}

type settingsModel struct {
	ui            *UI
	values        map[string]any
	tunnel        map[string]any
	cursor        int
	notice        string
	err           error
	loading       bool
	refreshing    bool
	actionRunning bool
}

func (ui *UI) liveSettings() error {
	EnableColors(ui.Out)
	model := settingsModel{ui: ui, loading: true}
	return ui.runTea(&model)
}

func (model *settingsModel) Init() tea.Cmd {
	return model.refreshCmd()
}

func (model *settingsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if model.err != nil && message.String() == "r" {
			model.err = nil
			model.loading = true
			return model, model.refreshCmd()
		}
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= 5 {
			if model.loading || model.actionRunning {
				return model, nil
			}
			model.cursor = index - 1
			return model, model.action(model.runAction)
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			model.cursor = moveIndex(model.cursor, 5, -1)
		case "down", "j":
			model.cursor = moveIndex(model.cursor, 5, 1)
		case "left", "h":
			model.cursor = cycleIndex(model.cursor, 5, -1)
		case "right", "l":
			model.cursor = cycleIndex(model.cursor, 5, 1)
		case "enter", " ":
			if model.loading || model.actionRunning {
				return model, nil
			}
			return model, model.action(model.runAction)
		}
	case settingsRefreshMsg:
		if model.actionRunning {
			return model, settingsRefresh()
		}
		if model.loading || model.refreshing {
			return model, settingsRefresh()
		}
		model.refreshing = true
		return model, model.refreshCmd()
	case settingsDataMsg:
		if message.err == nil {
			model.values, model.tunnel = message.values, message.tunnel
		}
		model.err, model.loading, model.refreshing = message.err, false, false
		return model, settingsRefresh()
	case settingsActionDoneMsg:
		model.actionRunning = false
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
			message.notice = ""
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.refreshing = true
			return model, model.refreshCmd()
		}
		return model, settingsRefresh()
	}
	return model, nil
}

func (model *settingsModel) View() string {
	if model.loading || model.actionRunning {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.settings")) + "\n\n" + mutedStyle.Render(model.ui.t("common.loading")))
	}
	if model.err != nil && len(model.values) == 0 && len(model.tunnel) == 0 {
		return model.ui.errorView(model.ui.t("menu.settings"), model.err)
	}
	columns := model.ui.responsiveColumns(2, 36)
	cardWidth := model.ui.responsiveCardWidth(2, 36)
	runtimeCard := innerCardStyle.Width(cardWidth).Render(cardTitleStyle.Render(model.ui.t("screen.runtime")) + "\n" +
		settingsActionItem(model.ui, 0, model.ui.t("settings.toggleRTK"), settingsEnabled(model.values, "rtkEnabled"), model.cursor == 0) + "\n" +
		settingsActionItem(model.ui, 1, model.ui.t("settings.toggleHeadroom"), settingsEnabled(model.values, "headroomEnabled"), model.cursor == 1) + "\n" +
		settingsActionItem(model.ui, 2, model.ui.t("settings.enableTunnel"), settingsEnabled(model.tunnel, "enabled"), model.cursor == 2) + "\n" +
		settingsActionItem(model.ui, 3, model.ui.t("settings.disableTunnel"), settingsEnabled(model.tunnel, "enabled"), model.cursor == 3))
	securityCard := innerCardStyle.Width(cardWidth).Render(cardTitleStyle.Render(model.ui.t("screen.security")) + "\n" +
		settingsActionItem(model.ui, 4, model.ui.t("settings.resetAuth"), false, model.cursor == 4))
	controls := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumns(
		model.ui.t("controls.toolsMove"), model.ui.t("controls.toolsSwitch"),
		model.ui.t("controls.languageSelect"), model.ui.t("controls.languageBack"),
	))
	if model.notice != "" {
		controls += "\n" + successStyle.Render(model.notice)
	}
	cards := model.ui.joinResponsiveCards([]string{runtimeCard, securityCard}, columns)
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.settings")) + "\n\n" + cards + "\n\n" + controls)
}

func settingsActionItem(ui *UI, index int, label string, enabled, selected bool) string {
	status := ""
	if index < 4 {
		status = " [" + ui.t(map[bool]string{true: "common.on", false: "common.off"}[enabled]) + "]"
	}
	text := strconv.Itoa(index+1) + "  " + label + status
	if selected {
		return focusStyle.Render(text)
	}
	return controlsStyle.Render(text)
}

func settingsEnabled(values map[string]any, key string) bool {
	value, ok := values[key].(bool)
	return ok && value
}

func (model *settingsModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
	model.actionRunning = true
	var notice string
	return tea.Exec(&endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		var err error
		notice, err = run(input, output)
		return err
	}}, func(err error) tea.Msg { return settingsActionDoneMsg{notice: notice, err: err} })
}

func (model *settingsModel) runAction(input io.Reader, output io.Writer) (string, error) {
	switch model.cursor {
	case 0:
		return model.ui.t("notice.rtkUpdated"), model.ui.request(http.MethodPut, "/api/settings", map[string]bool{"rtkEnabled": !settingsEnabled(model.values, "rtkEnabled")}, nil)
	case 1:
		return model.ui.t("notice.headroomUpdated"), model.ui.request(http.MethodPut, "/api/settings", map[string]bool{"headroomEnabled": !settingsEnabled(model.values, "headroomEnabled")}, nil)
	case 2:
		return model.ui.t("notice.tunnelEnabled"), model.ui.request(http.MethodPost, "/api/tunnel/enable", nil, nil)
	case 3:
		ok, err := model.ui.tuiConfirm(model.ui.t("confirm.disableTunnel"), input, output)
		if err != nil || !ok {
			return "", err
		}
		return model.ui.t("notice.tunnelDisabled"), model.ui.request(http.MethodPost, "/api/tunnel/disable", nil, nil)
	case 4:
		ok, err := model.ui.tuiConfirm(model.ui.t("confirm.resetAuth"), input, output)
		if err != nil || !ok {
			return "", err
		}
		return model.ui.t("notice.authReset"), model.ui.request(http.MethodPut, "/api/settings", map[string]string{"authMode": "password"}, nil)
	}
	return "", nil
}

func (model *settingsModel) refresh() {
	var values map[string]any
	if err := model.ui.request(http.MethodGet, "/api/settings", nil, &values); err != nil {
		model.err = err
		return
	}
	var tunnel map[string]any
	if err := model.ui.request(http.MethodGet, "/api/tunnel/status", nil, &tunnel); err != nil {
		model.err = err
		return
	}
	model.values, model.tunnel, model.err = values, tunnel, nil
}

func (model *settingsModel) refreshCmd() tea.Cmd {
	ui := model.ui
	return func() tea.Msg {
		var values map[string]any
		if err := ui.request(http.MethodGet, "/api/settings", nil, &values); err != nil {
			return settingsDataMsg{err: err}
		}
		var tunnel map[string]any
		if err := ui.request(http.MethodGet, "/api/tunnel/status", nil, &tunnel); err != nil {
			return settingsDataMsg{err: err}
		}
		return settingsDataMsg{values: values, tunnel: tunnel}
	}
}

func settingsRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return settingsRefreshMsg{} })
}
