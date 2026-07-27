package tui

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type statisticsRefreshMsg struct{}

type statisticsModel struct {
	ui           *UI
	period       int
	chartCursor  int
	focusCurrent bool
	stats        statisticsPayload
	chart        []statisticsPoint
	err          error
}

type statisticsPayload struct {
	TotalRequests         int64               `json:"totalRequests"`
	TotalPromptTokens     int64               `json:"totalPromptTokens"`
	TotalCompletionTokens int64               `json:"totalCompletionTokens"`
	TotalCachedTokens     int64               `json:"totalCachedTokens"`
	TotalCost             float64             `json:"totalCost"`
	ByProvider            map[string]int      `json:"byProvider"`
	ByModel               map[string]int      `json:"byModel"`
	RecentRequests        []statisticsRequest `json:"recentRequests"`
}

type statisticsRequest struct {
	Timestamp        string `json:"timestamp"`
	Model            string `json:"model"`
	Provider         string `json:"provider"`
	PromptTokens     int64  `json:"promptTokens"`
	CompletionTokens int64  `json:"completionTokens"`
	Status           string `json:"status"`
}

type statisticsPoint struct {
	Label  string `json:"label"`
	Tokens int64  `json:"tokens"`
}

var statisticsPeriods = []string{"today", "24h", "7d", "30d", "60d"}

func (ui *UI) liveStatistics() error {
	EnableColors(ui.Out)
	model := statisticsModel{ui: ui, focusCurrent: true}
	model.refresh()
	return ui.runTea(&model)
}

func (model *statisticsModel) Init() tea.Cmd { return statisticsRefresh() }

func (model *statisticsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= len(statisticsPeriods) {
			model.period = index - 1
			model.focusCurrent = true
			model.refresh()
			return model, statisticsRefresh()
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "left", "h":
			if model.period > 0 {
				model.period--
				model.focusCurrent = true
				model.refresh()
			}
		case "right", "l":
			if model.period+1 < len(statisticsPeriods) {
				model.period++
				model.focusCurrent = true
				model.refresh()
			}
		case "up", "k":
			if model.chartCursor > 0 {
				model.chartCursor--
			}
		case "down", "j":
			if model.chartCursor+1 < len(model.chart) {
				model.chartCursor++
			}
		case "r", "enter", " ":
			model.refresh()
		}
	case statisticsRefreshMsg:
		model.refresh()
		return model, statisticsRefresh()
	}
	return model, nil
}

func (model *statisticsModel) View() string {
	if model.err != nil {
		return outerCardStyle.Render(cardTitleStyle.Render(model.ui.t("menu.statistics")) + "\n\n" + model.err.Error() + "\n\n" + mutedStyle.Render("Press q or Esc to go back."))
	}
	periods := make([]string, len(statisticsPeriods))
	for index, period := range statisticsPeriods {
		style := mutedStyle.Padding(0, 1)
		if index == model.period {
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1)
		}
		periods[index] = style.Render(periodLabel(period))
	}
	banner := lipgloss.NewStyle().Width(78).Align(lipgloss.Center).Render(gradientText(cliBanner))
	controls := innerCardStyle.Render(cardTitleStyle.Render("Controls")+"\n"+lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("←→/hl period")),
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("↑↓/jk token cursor"))),
		lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("r refresh")),
			lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("q back")),
		))
	breakdowns := lipgloss.JoinHorizontal(lipgloss.Top, model.breakdownCard("By Provider", model.stats.ByProvider), model.breakdownCard("By Model", model.stats.ByModel))
	activity := lipgloss.JoinHorizontal(lipgloss.Top, model.chartCard(), model.recentCard())
	return outerCardStyle.Render(banner + "\n\n" + cardTitleStyle.Render(model.ui.t("menu.statistics")) + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, periods...) + "\n\n" + model.overviewCard() + "\n\n" + breakdowns + "\n\n" + activity + "\n\n" + controls)
}

func (model *statisticsModel) overviewCard() string {
	rows := []string{
		statisticsLine("Requests", formatInt(model.stats.TotalRequests), "Prompt tokens", formatInt(model.stats.TotalPromptTokens)),
		statisticsLine("Completion tokens", formatInt(model.stats.TotalCompletionTokens), "Cached tokens", formatInt(model.stats.TotalCachedTokens)),
		statisticsLine("Total tokens", formatInt(model.stats.TotalPromptTokens+model.stats.TotalCompletionTokens), "Estimated cost", fmt.Sprintf("$%.4f", model.stats.TotalCost)),
	}
	return innerCardStyle.Render(cardTitleStyle.Render("Overview") + "\n" + strings.Join(rows, "\n"))
}

func statisticsLine(leftLabel, leftValue, rightLabel, rightValue string) string {
	column := lipgloss.NewStyle().Width(30)
	return lipgloss.JoinHorizontal(lipgloss.Top, column.Render(leftLabel+": "+leftValue), column.Render(rightLabel+": "+rightValue))
}

func (model *statisticsModel) breakdownCard(title string, values map[string]int) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return values[keys[left]] > values[keys[right]] })
	rows := make([]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, truncateText(key, 24)+": "+formatInt(int64(values[key])))
	}
	if len(rows) == 0 {
		rows = append(rows, "No usage recorded yet.")
	}
	return innerCardStyle.Width(31).Render(cardTitleStyle.Render(title) + "\n" + strings.Join(rows, "\n"))
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func (model *statisticsModel) chartCard() string {
	max := int64(1)
	for _, point := range model.chart {
		if point.Tokens > max {
			max = point.Tokens
		}
	}
	start, end := 0, len(model.chart)
	if end-start > 5 {
		start = model.chartCursor - 4
		if start < 0 {
			start = 0
		}
		if start+5 > end {
			start = end - 5
		}
	}
	rows := make([]string, 5)
	for index := range rows {
		chartIndex := start + index
		if chartIndex >= end {
			rows[index] = " "
			continue
		}
		point := model.chart[chartIndex]
		width := int(point.Tokens * 10 / max)
		line := fmt.Sprintf("✦ %-8s %-10s %s", point.Label, strings.Repeat("█", width), formatInt(point.Tokens))
		if chartIndex == model.chartCursor {
			line = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Render(line)
		}
		rows[index] = line
	}
	return innerCardStyle.Width(31).Render(cardTitleStyle.Render("Token Usage") + "\n" + strings.Join(rows, "\n"))
}

func (model *statisticsModel) recentCard() string {
	rows := make([]string, 0, len(model.stats.RecentRequests))
	for _, request := range model.stats.RecentRequests {
		status := request.Status
		if status == "" {
			status = "ok"
		}
		rows = append(rows, fmt.Sprintf("%-5s %-10s %-7s %s/%s", status, truncateText(request.Model, 10), truncateText(request.Provider, 7), formatInt(request.PromptTokens), formatInt(request.CompletionTokens)))
	}
	if len(rows) == 0 {
		rows = append(rows, "No requests yet.")
	}
	return innerCardStyle.Width(31).Render(cardTitleStyle.Render("Recent Requests") + "\n" + strings.Join(rows, "\n"))
}

func (model *statisticsModel) refresh() {
	period := statisticsPeriods[model.period]
	var stats statisticsPayload
	if err := model.ui.request(http.MethodGet, "/api/usage/stats?period="+url.QueryEscape(period), nil, &stats); err != nil {
		model.err = err
		return
	}
	var chart []statisticsPoint
	if err := model.ui.request(http.MethodGet, "/api/usage/chart?period="+url.QueryEscape(period), nil, &chart); err != nil {
		model.err = err
		return
	}
	model.stats, model.chart, model.err = stats, chart, nil
	if model.focusCurrent {
		if len(model.chart) == 0 {
			model.chartCursor = 0
		} else if period == "today" {
			model.chartCursor = time.Now().Hour()
			if model.chartCursor >= len(model.chart) {
				model.chartCursor = len(model.chart) - 1
			}
		} else {
			model.chartCursor = len(model.chart) - 1
		}
		model.focusCurrent = false
	} else if model.chartCursor >= len(model.chart) {
		model.chartCursor = 0
		if len(model.chart) > 0 {
			model.chartCursor = len(model.chart) - 1
		}
	}
}

func periodLabel(period string) string {
	return map[string]string{"today": "Today", "24h": "24h", "7d": "7d", "30d": "30d", "60d": "60d"}[period]
}

func formatInt(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	text := strconv.FormatInt(value, 10)
	for index := len(text) - 3; index > 0; index -= 3 {
		text = text[:index] + "," + text[index:]
	}
	if negative {
		return "-" + text
	}
	return text
}

func statisticsRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return statisticsRefreshMsg{} })
}
