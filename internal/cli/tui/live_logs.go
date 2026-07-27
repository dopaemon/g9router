package tui

import (
	"fmt"
	"net/http"
	"regexp"
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
	apiErr  error
	httpErr error
	detail  string
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
		if model.detail != "" {
			switch message.String() {
			case "q", "esc", "enter":
				model.detail = ""
			}
			return model, nil
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "tab", "right", "l":
			model.tab = cycleIndex(model.tab, 2, 1)
			model.cursor = 0
		case "shift+tab", "left", "h":
			model.tab = cycleIndex(model.tab, 2, -1)
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
			if message.String() == "enter" && model.rowCount() > 0 {
				model.detail = model.detailForCursor()
				return model, nil
			}
			model.refresh()
		}
	case logsRefreshMsg:
		model.refresh()
		return model, logsRefresh()
	}
	return model, nil
}

func (model *logsModel) View() string {
	if model.detail != "" {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("logs.detail")) + "\n\n" + model.ui.innerStyle().Render(model.detail) + "\n\n" + mutedStyle.Render(model.ui.t("logs.detailControls")))
	}
	tabs := []string{model.ui.t("logs.apiAgent"), model.ui.t("logs.http")}
	for index := range tabs {
		if index == model.tab {
			tabs[index] = focusStyle.Render(tabs[index])
		} else {
			tabs[index] = mutedStyle.Render(tabs[index])
		}
	}
	content := model.ui.innerStyle().Render(cardTitleStyle.Render(tabs[model.tab]) + "\n" + model.currentContent())
	controls := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + mutedStyle.Render(model.ui.t("logs.controls")))
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.logs")) + "\n\n" + strings.Join(tabs, "  ") + "\n\n" + content + "\n\n" + controls)
}

func (model *logsModel) detailForCursor() string {
	if model.tab == 1 {
		return redactLogText(model.httpLog[model.cursor])
	}
	entry := model.apiLogs[model.cursor]
	return redactLogText(fmt.Sprintf("%s: %s\n%s: %s\n%s: %s\n%s: %s\n%s: %d\n%s: %d", model.ui.t("logs.timestamp"), entry.Timestamp, model.ui.t("logs.status"), entry.Status, model.ui.t("logs.provider"), entry.Provider, model.ui.t("logs.model"), entry.Model, model.ui.t("logs.inputTokens"), entry.Input, model.ui.t("logs.outputTokens"), entry.Output))
}

var logSecretPattern = regexp.MustCompile(`(?i)(bearer\s+|token=|secret=|password=|api[_-]?key=)([^\s,]+)`)

func redactLogText(value string) string {
	return logSecretPattern.ReplaceAllString(value, "$1••••")
}

func (model *logsModel) currentError() error {
	if model.tab == 1 {
		return model.httpErr
	}
	return model.apiErr
}

func (model *logsModel) currentContent() string {
	if err := model.currentError(); err != nil {
		message := errorStyle.Render(model.ui.t("common.error") + ": " + model.ui.errorSummary(err))
		if model.rowCount() > 0 {
			message += "\n" + mutedStyle.Render(model.ui.t("logs.stale"))
		}
		return message
	}
	return model.rows()
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
			rows[index] = model.logRow(index, truncateText(value, model.ui.innerWidth()-2))
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
		rows[index] = model.logRow(index, model.formatAPILog(timestamp, status, entry))
	}
	return rows
}

func (model *logsModel) formatAPILog(timestamp, status string, entry apiLogEntry) string {
	width := model.ui.innerWidth() - 2
	line := fmt.Sprintf("%-8s %-7s %-18s %-24s %d/%d", timestamp, status, truncateText(entry.Provider, 18), truncateText(entry.Model, 24), entry.Input, entry.Output)
	if width < 72 {
		line = fmt.Sprintf("%s %-7s %s/%s %d/%d", timestamp, status, truncateText(entry.Provider, 12), truncateText(entry.Model, 18), entry.Input, entry.Output)
	}
	return truncateText(line, width)
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
	apiErr := model.ui.request(http.MethodGet, "/api/usage/logs", nil, &apiLogs)
	var payload struct {
		Logs []string `json:"logs"`
	}
	httpErr := model.ui.request(http.MethodGet, "/api/translator/console-logs", nil, &payload)
	if apiErr == nil {
		model.apiLogs = apiLogs
	}
	if httpErr == nil {
		model.httpLog = payload.Logs
	}
	model.apiErr, model.httpErr = apiErr, httpErr
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
