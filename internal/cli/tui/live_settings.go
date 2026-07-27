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
	runtimeCard := innerCardStyle.Width(cardWidth).Render(cardTitleStyle.Render(model.ui.t("screen.runtime")) + "\n" +
		settingsActionItem(0, "Toggle RTK", settingsEnabled(model.values, "rtkEnabled"), model.cursor == 0) + "\n" +
		settingsActionItem(1, "Toggle Headroom", settingsEnabled(model.values, "headroomEnabled"), model.cursor == 1) + "\n" +
		settingsActionItem(2, "Enable Tunnel", settingsEnabled(model.tunnel, "enabled"), model.cursor == 2) + "\n" +
		settingsActionItem(3, "Disable Tunnel", settingsEnabled(model.tunnel, "enabled"), model.cursor == 3))
	securityCard := innerCardStyle.Width(cardWidth).Render(cardTitleStyle.Render(model.ui.t("screen.security")) + "\n" +
		settingsActionItem(4, "Reset auth mode", false, model.cursor == 4) + "\n" +
		settingsActionItem(5, "Reset password", false, model.cursor == 5) + "\n\n" + mutedStyle.Render("Password: "+model.ui.t(map[bool]string{true: "settings.passwordConfigured", false: "settings.passwordMissing"}[settingsEnabled(model.values, "hasPassword")])))
	controls := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
		model.ui.controlStyle().Render(mutedStyle.Render(model.ui.t("controls.toolsMoveSwitch"))),
		model.ui.controlStyle().Render(mutedStyle.Render(model.ui.t("controls.languageSelectBack"))),
	))
	if model.notice != "" {
		controls += "\n" + successStyle.Render(model.notice)
	}
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.settings")) + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, runtimeCard, securityCard) + "\n\n" + controls)
}

func settingsActionItem(index int, label string, enabled, selected bool) string {
	status := ""
	if label == "Toggle RTK" || label == "Toggle Headroom" || label == "Enable Tunnel" || label == "Disable Tunnel" {
		status = " [" + map[bool]string{true: "ON", false: "OFF"}[enabled] + "]"
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
		return "RTK updated", model.ui.request(http.MethodPut, "/api/settings", map[string]bool{"rtkEnabled": !settingsEnabled(model.values, "rtkEnabled")}, nil)
	case 1:
		return "Headroom updated", model.ui.request(http.MethodPut, "/api/settings", map[string]bool{"headroomEnabled": !settingsEnabled(model.values, "headroomEnabled")}, nil)
	case 2:
		return "Tunnel enabled", model.ui.request(http.MethodPost, "/api/tunnel/enable", nil, nil)
	case 3:
		ok, err := model.ui.tuiConfirm("Disable the tunnel?", input, output)
		if err != nil || !ok {
			return "", err
		}
		return "Tunnel disabled", model.ui.request(http.MethodPost, "/api/tunnel/disable", nil, nil)
	case 4:
		ok, err := model.ui.tuiConfirm("Switch authentication to password mode?", input, output)
		if err != nil || !ok {
			return "", err
		}
		return "Auth mode reset", model.ui.request(http.MethodPut, "/api/settings", map[string]string{"authMode": "password"}, nil)
	case 5:
		ok, err := model.ui.tuiConfirm("Reset the admin password?", input, output)
		if err != nil || !ok {
			return "", err
		}
		return "Password reset", model.ui.request(http.MethodPost, "/api/auth/reset-password", nil, nil)
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
