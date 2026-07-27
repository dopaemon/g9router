package tui

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type settingsRefreshMsg struct{}
type settingsActionDoneMsg struct {
	notice string
	err    error
}

type settingsModel struct {
	ui     *UI
	values map[string]any
	tunnel map[string]any
	cursor int
	notice string
	err    error
}

func (ui *UI) liveSettings() error {
	EnableColors(ui.Out)
	model := settingsModel{ui: ui}
	model.refresh()
	return ui.runTea(&model)
}

func (model *settingsModel) Init() tea.Cmd { return settingsRefresh() }

func (model *settingsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if model.err != nil && message.String() == "r" {
			model.err = nil
			model.refresh()
			return model, settingsRefresh()
		}
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= 6 {
			model.cursor = index - 1
			return model, model.action(model.runAction)
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			if model.cursor > 0 {
				model.cursor--
			}
		case "down", "j":
			if model.cursor+1 < 6 {
				model.cursor++
			}
		case "left", "h":
			if model.cursor >= 4 {
				model.cursor = 0
			}
		case "right", "l":
			if model.cursor < 4 {
				model.cursor = 4
			}
		case "enter", " ":
			return model, model.action(model.runAction)
		}
	case settingsRefreshMsg:
		model.refresh()
		return model, settingsRefresh()
	case settingsActionDoneMsg:
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.refresh()
		}
		return model, settingsRefresh()
	}
	return model, nil
}

func (model *settingsModel) View() string {
	if model.err != nil {
		return model.ui.errorView(model.ui.t("menu.settings"), model.err)
	}
	cardWidth := model.ui.columnWidth(2)
	if model.ui.compact() {
		cardWidth = model.ui.innerWidth()
	}
	runtimeCard := innerCardStyle.Width(cardWidth).Render(cardTitleStyle.Render(model.ui.t("screen.runtime")) + "\n" +
		settingsActionItem(model.ui, 0, model.ui.t("settings.toggleRTK"), settingsEnabled(model.values, "rtkEnabled"), model.cursor == 0) + "\n" +
		settingsActionItem(model.ui, 1, model.ui.t("settings.toggleHeadroom"), settingsEnabled(model.values, "headroomEnabled"), model.cursor == 1) + "\n" +
		settingsActionItem(model.ui, 2, model.ui.t("settings.enableTunnel"), settingsEnabled(model.tunnel, "enabled"), model.cursor == 2) + "\n" +
		settingsActionItem(model.ui, 3, model.ui.t("settings.disableTunnel"), settingsEnabled(model.tunnel, "enabled"), model.cursor == 3))
	securityCard := innerCardStyle.Width(cardWidth).Render(cardTitleStyle.Render(model.ui.t("screen.security")) + "\n" +
		settingsActionItem(model.ui, 4, model.ui.t("settings.resetAuth"), false, model.cursor == 4) + "\n" +
		settingsActionItem(model.ui, 5, model.ui.t("settings.resetPassword"), false, model.cursor == 5) + "\n\n" + mutedStyle.Render(model.ui.t("settings.passwordLabel")+": "+model.ui.t(map[bool]string{true: "settings.passwordConfigured", false: "settings.passwordMissing"}[settingsEnabled(model.values, "hasPassword")])))
	controls := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
		model.ui.controlStyle().Render(mutedStyle.Render(model.ui.t("controls.toolsMoveSwitch"))),
		model.ui.controlStyle().Render(mutedStyle.Render(model.ui.t("controls.languageSelectBack"))),
	))
	if model.notice != "" {
		controls += "\n" + successStyle.Render(model.notice)
	}
	cards := lipgloss.JoinHorizontal(lipgloss.Top, runtimeCard, securityCard)
	if model.ui.compact() {
		cards = lipgloss.JoinVertical(lipgloss.Left, runtimeCard, securityCard)
	}
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
	case 5:
		ok, err := model.ui.tuiConfirm(model.ui.t("confirm.resetPassword"), input, output)
		if err != nil || !ok {
			return "", err
		}
		return model.ui.t("notice.passwordReset"), model.ui.request(http.MethodPost, "/api/auth/reset-password", nil, nil)
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

func settingsRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return settingsRefreshMsg{} })
}
