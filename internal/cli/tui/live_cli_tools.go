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
	_, err := tea.NewProgram(&model, tea.WithInput(ui.In), tea.WithOutput(ui.Out)).Run()
	return err
}

func (model *cliToolsModel) Init() tea.Cmd { return cliToolsRefresh() }

func (model *cliToolsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= len(cliToolOrder) {
			model.cursor = index - 1
			return model, model.action(model.show)
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			if model.cursor >= 2 {
				model.cursor -= 2
			}
		case "down", "j":
			if model.cursor+2 < len(cliToolOrder) {
				model.cursor += 2
			}
		case "left", "h":
			if model.cursor%2 == 1 {
				model.cursor--
			}
		case "right", "l":
			if model.cursor%2 == 0 && model.cursor+1 < len(cliToolOrder) {
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
		return outerCardStyle.Render(cardTitleStyle.Render(model.ui.t("menu.cliTools")) + "\n\n" + model.err.Error() + "\n\n" + mutedStyle.Render("Press q or Esc to go back."))
	}
	cards := make([]string, 0, (len(cliToolOrder)+1)/2)
	column := lipgloss.NewStyle().Width(38)
	rowCount := (len(cliToolOrder) + 1) / 2
	startRow := model.cursor / 2
	if startRow > 2 {
		startRow -= 2
	}
	if startRow+3 > rowCount {
		startRow = rowCount - 3
	}
	if startRow < 0 {
		startRow = 0
	}
	for row := startRow; row < rowCount && row < startRow+3; row++ {
		index := row * 2
		left := cliToolCard(index, cliToolOrder[index], model.statuses[cliToolOrder[index]], index == model.cursor)
		right := ""
		if index+1 < len(cliToolOrder) {
			right = cliToolCard(index+1, cliToolOrder[index+1], model.statuses[cliToolOrder[index+1]], index+1 == model.cursor)
		}
		cards = append(cards, lipgloss.JoinHorizontal(lipgloss.Top, column.Render(left), column.Render(right)))
	}
	controls := innerCardStyle.Render(cardTitleStyle.Render("Controls") + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("↑↓/jk move  ←→/hl switch")),
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("Enter/s show  r reset  q back")),
	))
	if model.notice != "" {
		controls += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80")).Render(model.notice)
	}
	banner := lipgloss.NewStyle().Width(78).Align(lipgloss.Center).Render(gradientText(cliBanner))
	return outerCardStyle.Render(banner + "\n\n" + cardTitleStyle.Render(model.ui.t("menu.cliTools")) + "\n\n" + lipgloss.JoinVertical(lipgloss.Left, cards...) + "\n\n" + controls)
}

func cliToolCard(index int, id string, status cliToolStatus, selected bool) string {
	label := cliToolLabels[id]
	state := "Not installed"
	color := "#94A3B8"
	if status.Installed && !status.Configured {
		state, color = "Not configured", "#FBBF24"
	}
	if status.Configured {
		state, color = "Connected", "#4ADE80"
	}
	title := fmt.Sprintf("%d  %s", index+1, label)
	if selected {
		title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1).Render(title)
	} else {
		title = cardTitleStyle.Render(title)
	}
	return innerCardStyle.Width(31).Padding(0, 1).Render(title + "\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(state))
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
	ok, err := confirmHuh(input, output, "Reset "+cliToolLabels[id]+" settings?", func(form *huh.Form) error {
		return model.ui.runHuhIO(form, input, output)
	})
	if err != nil || !ok {
		return "", err
	}
	return "Settings reset", model.ui.request(http.MethodDelete, cliToolPaths[id], nil, nil)
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
