package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type cliToolsRefreshMsg struct{}
type cliToolsActionDoneMsg struct {
	notice string
	err    error
}

type cliToolStatus struct {
	Installed  bool `json:"installed"`
	Configured bool `json:"configured"`
	Available  bool `json:"available"`
}

type cliToolsModel struct {
	ui       *UI
	cursor   int
	statuses map[string]cliToolStatus
	notice   string
	err      error
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
	model := cliToolsModel{ui: ui}
	model.refresh()
	return ui.runTea(&model)
}

func (model *cliToolsModel) Init() tea.Cmd { return cliToolsRefresh() }

func (model *cliToolsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if model.err != nil && message.String() == "r" {
			model.err = nil
			model.refresh()
			return model, cliToolsRefresh()
		}
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= len(cliToolOrder) {
			model.cursor = index - 1
			return model, model.action(model.show)
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			step := 2
			if model.ui.compact() {
				step = 1
			}
			if model.cursor >= step {
				model.cursor -= step
			}
		case "down", "j":
			step := 2
			if model.ui.compact() {
				step = 1
			}
			if model.cursor+step < len(cliToolOrder) {
				model.cursor += step
			}
		case "left", "h":
			if !model.ui.compact() && model.cursor%2 == 1 {
				model.cursor--
			}
		case "right", "l":
			if !model.ui.compact() && model.cursor%2 == 0 && model.cursor+1 < len(cliToolOrder) {
				model.cursor++
			}
		case "enter", " ", "s":
			return model, model.action(model.show)
		case "r":
			return model, model.action(model.reset)
		}
	case cliToolsRefreshMsg:
		model.refresh()
		return model, cliToolsRefresh()
	case cliToolsActionDoneMsg:
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.refresh()
		}
		return model, cliToolsRefresh()
	}
	return model, nil
}

func (model *cliToolsModel) View() string {
	if model.err != nil {
		return model.ui.errorView(model.ui.t("menu.cliTools"), model.err)
	}
	cards := make([]string, 0, (len(cliToolOrder)+1)/2)
	if model.ui.compact() {
		visible := max(1, min(5, model.ui.viewportHeight(8, 10)/2))
		start, end := viewportWindow(model.cursor, len(cliToolOrder), visible)
		for index := start; index < end; index++ {
			cards = append(cards, lipgloss.NewStyle().Width(model.ui.innerWidth()).Render(cliToolCard(model.ui, index, cliToolOrder[index], model.statuses[cliToolOrder[index]], index == model.cursor)))
		}
	} else {
		column := lipgloss.NewStyle().Width(model.ui.columnWidth(2))
		rowCount := (len(cliToolOrder) + 1) / 2
		visibleRows := max(1, min(3, model.ui.viewportHeight(8, 6)/2))
		startRow, endRow := viewportWindow(model.cursor/2, rowCount, visibleRows)
		for row := startRow; row < endRow; row++ {
			index := row * 2
			left := cliToolCard(model.ui, index, cliToolOrder[index], model.statuses[cliToolOrder[index]], index == model.cursor)
			right := ""
			if index+1 < len(cliToolOrder) {
				right = cliToolCard(model.ui, index+1, cliToolOrder[index+1], model.statuses[cliToolOrder[index+1]], index+1 == model.cursor)
			}
			cards = append(cards, lipgloss.JoinHorizontal(lipgloss.Top, column.Render(left), column.Render(right)))
		}
	}
	controls := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
		model.ui.controlStyle().Render(mutedStyle.Render(model.ui.t("controls.toolsMoveSwitch"))),
		model.ui.controlStyle().Render(mutedStyle.Render(model.ui.t("controls.toolsShowResetBack"))),
	))
	if model.notice != "" {
		controls += "\n" + successStyle.Render(model.notice)
	}
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.cliTools")) + "\n\n" + lipgloss.JoinVertical(lipgloss.Left, cards...) + "\n\n" + controls)
}

func cliToolCard(ui *UI, index int, id string, status cliToolStatus, selected bool) string {
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
	return innerCardStyle.Padding(0, 1).Render(title + "\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(state))
}

func (model *cliToolsModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
	var notice string
	return tea.Exec(&endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		var err error
		notice, err = run(input, output)
		return err
	}}, func(err error) tea.Msg { return cliToolsActionDoneMsg{notice: notice, err: err} })
}

func (model *cliToolsModel) show(input io.Reader, output io.Writer) (string, error) {
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

func cliToolsRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return cliToolsRefreshMsg{} })
}
