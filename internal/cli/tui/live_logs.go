package tui

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type logsRefreshMsg struct{}

type logsModel struct {
	ui      *UI
	tab     int
	cursor  int
	apiLogs []apiLogEntry
	httpLog []string
	err     error
}

type apiLogEntry struct {
	Timestamp string `json:"timestamp"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Status    string `json:"status"`
	Input     int64  `json:"inputTokens"`
	Output    int64  `json:"outputTokens"`
}

func (ui *UI) liveLogs() error {
	model := logsModel{ui: ui}
	model.refresh()
	return ui.runTea(&model)
}

func (model *logsModel) Init() tea.Cmd { return logsRefresh() }

func (model *logsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "tab", "right", "l":
			model.tab = (model.tab + 1) % 2
			model.cursor = 0
		case "shift+tab", "left", "h":
			model.tab = (model.tab + 1) % 2
			model.cursor = 0
		case "up", "k":
			if model.cursor > 0 {
				model.cursor--
			}
		case "down", "j":
			if model.cursor+1 < model.rowCount() {
				model.cursor++
			}
		case "r", "enter", " ":
			model.refresh()
		}
	case logsRefreshMsg:
		model.refresh()
		return model, logsRefresh()
	}
	return model, nil
}

func (model *logsModel) View() string {
	tabs := []string{model.ui.t("logs.apiAgent"), model.ui.t("logs.http")}
	for index := range tabs {
		if index == model.tab {
			tabs[index] = focusStyle.Render(tabs[index])
		} else {
			tabs[index] = mutedStyle.Render(tabs[index])
		}
	}
	content := model.ui.innerStyle().Render(cardTitleStyle.Render(tabs[model.tab]) + "\n" + model.rows())
	controls := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + mutedStyle.Render(model.ui.t("logs.controls")))
	if model.err != nil {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.logs")) + "\n\n" + errorStyle.Render(model.ui.t("common.error")+": "+model.err.Error()) + "\n\n" + mutedStyle.Render(model.ui.t("common.retryBack")))
	}
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.logs")) + "\n\n" + strings.Join(tabs, "  ") + "\n\n" + content + "\n\n" + controls)
}

func (model *logsModel) rows() string {
	if model.rowCount() == 0 {
		return mutedStyle.Render(model.ui.t("logs.empty"))
	}
	rows := model.currentRows()
	start := model.cursor - 9
	if start < 0 {
		start = 0
	}
	if start+10 > len(rows) {
		start = len(rows) - 10
		if start < 0 {
			start = 0
		}
	}
	end := start + 10
	if end > len(rows) {
		end = len(rows)
	}
	return strings.Join(rows[start:end], "\n")
}

func (model *logsModel) currentRows() []string {
	if model.tab == 1 {
		rows := make([]string, len(model.httpLog))
		for index, value := range model.httpLog {
			rows[index] = model.logRow(index, value)
		}
		return rows
	}
	rows := make([]string, len(model.apiLogs))
	for index, entry := range model.apiLogs {
		timestamp := entry.Timestamp
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			timestamp = parsed.Local().Format("15:04:05")
		}
		status := entry.Status
		if status == "" {
			status = "ok"
		}
		rows[index] = model.logRow(index, fmt.Sprintf("%-8s %-7s %-18s %-24s %d/%d", timestamp, status, truncateText(entry.Provider, 18), truncateText(entry.Model, 24), entry.Input, entry.Output))
	}
	return rows
}

func (model *logsModel) logRow(index int, value string) string {
	if index == model.cursor {
		return focusStyle.Render(value)
	}
	return controlsStyle.Render(value)
}

func (model *logsModel) rowCount() int {
	if model.tab == 1 {
		return len(model.httpLog)
	}
	return len(model.apiLogs)
}

func (model *logsModel) refresh() {
	var apiLogs []apiLogEntry
	if err := model.ui.request(http.MethodGet, "/api/usage/logs", nil, &apiLogs); err != nil {
		model.err = err
		return
	}
	var payload struct {
		Logs []string `json:"logs"`
	}
	if err := model.ui.request(http.MethodGet, "/api/translator/console-logs", nil, &payload); err != nil {
		model.err = err
		return
	}
	model.apiLogs, model.httpLog, model.err = apiLogs, payload.Logs, nil
	if model.cursor >= model.rowCount() {
		model.cursor = model.rowCount() - 1
		if model.cursor < 0 {
			model.cursor = 0
		}
	}
}

func logsRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return logsRefreshMsg{} })
}
