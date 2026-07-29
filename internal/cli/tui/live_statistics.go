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
	"github.com/charmbracelet/x/ansi"
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
	loading      bool
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
	model := statisticsModel{ui: ui, focusCurrent: true, loading: true}
	return ui.runTea(&model)
}

func (model *statisticsModel) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return statisticsRefreshMsg{} }, statisticsRefresh())
}

func (model *statisticsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if model.err != nil && message.String() == "r" {
			model.err = nil
			model.refresh()
			return model, statisticsRefresh()
		}
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
			previous := moveIndex(model.period, len(statisticsPeriods), -1)
			if previous != model.period {
				model.period = previous
				model.focusCurrent = true
				model.refresh()
			}
		case "right", "l":
			next := moveIndex(model.period, len(statisticsPeriods), 1)
			if next != model.period {
				model.period = next
				model.focusCurrent = true
				model.refresh()
			}
		case "up", "k":
			model.chartCursor = moveIndex(model.chartCursor, len(model.chart), -1)
		case "down", "j":
			model.chartCursor = moveIndex(model.chartCursor, len(model.chart), 1)
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
	if model.loading {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.statistics")) + "\n\n" + mutedStyle.Render(model.ui.t("common.loading")))
	}
	periods := make([]string, len(statisticsPeriods))
	for index, period := range statisticsPeriods {
		style := mutedStyle.Padding(0, 1)
		if index == model.period {
			style = focusStyle
		}
		periods[index] = style.Render(model.ui.t(periodLabel(period)))
	}
	controls := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumns(
		model.ui.t("controls.period"), model.ui.t("controls.tokenCursor"),
		model.ui.t("controls.refresh"), model.ui.t("controls.back"),
	))
	breakdowns := lipgloss.JoinHorizontal(lipgloss.Top, model.breakdownCard(model.ui.t("screen.byProvider"), model.stats.ByProvider), model.breakdownCard(model.ui.t("screen.byModel"), model.stats.ByModel))
	activity := lipgloss.JoinHorizontal(lipgloss.Top, model.chartCard(), model.recentCard())
	if model.ui.compact() {
		breakdowns = lipgloss.JoinVertical(lipgloss.Left, model.breakdownCard(model.ui.t("screen.byProvider"), model.stats.ByProvider), model.breakdownCard(model.ui.t("screen.byModel"), model.stats.ByModel))
		activity = lipgloss.JoinVertical(lipgloss.Left, model.chartCard(), model.recentCard())
	}
	periodView := lipgloss.JoinHorizontal(lipgloss.Top, periods...)
	if model.ui.compact() {
		periodView = lipgloss.JoinVertical(lipgloss.Left, periods...)
	}
	content := cardTitleStyle.Render(model.ui.t("menu.statistics")) + "\n\n" + periodView + "\n\n" + model.overviewCard() + "\n\n" + breakdowns + "\n\n" + activity + "\n\n" + controls
	if model.err != nil {
		content += "\n\n" + errorStyle.Render("ERROR: "+model.ui.errorSummary(model.err))
	}
	return model.ui.outerStyle().Render(content)
}

func (model *statisticsModel) overviewCard() string {
	rows := []string{
		model.statisticsLine(model.ui.t("stats.requests"), formatInt(model.stats.TotalRequests), model.ui.t("stats.promptTokens"), formatInt(model.stats.TotalPromptTokens)),
		model.statisticsLine(model.ui.t("stats.completionTokens"), formatInt(model.stats.TotalCompletionTokens), model.ui.t("stats.cachedTokens"), formatInt(model.stats.TotalCachedTokens)),
		model.statisticsLine(model.ui.t("stats.totalTokens"), formatInt(model.stats.TotalPromptTokens+model.stats.TotalCompletionTokens), model.ui.t("stats.estimatedCost"), fmt.Sprintf("$%.4f", model.stats.TotalCost)),
	}
	return model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("screen.overview")) + "\n" + strings.Join(rows, "\n"))
}

func (model *statisticsModel) statisticsLine(leftLabel, leftValue, rightLabel, rightValue string) string {
	if model.ui.compact() {
		return leftLabel + ": " + leftValue + "\n" + rightLabel + ": " + rightValue
	}
	column := lipgloss.NewStyle().Width(model.ui.columnWidth(2))
	return lipgloss.JoinHorizontal(lipgloss.Top, column.Render(leftLabel+": "+leftValue), column.Render(rightLabel+": "+rightValue))
}

func (model *statisticsModel) breakdownCard(title string, values map[string]int) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if values[keys[left]] == values[keys[right]] {
			return keys[left] < keys[right]
		}
		return values[keys[left]] > values[keys[right]]
	})
	rows := make([]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, truncateText(key, 24)+": "+formatInt(int64(values[key])))
	}
	if len(rows) == 0 {
		rows = append(rows, model.ui.t("stats.noUsage"))
	}
	width := model.ui.columnWidth(2)
	if model.ui.compact() {
		width = model.ui.innerWidth()
	}
	return model.ui.innerStyle(width).Render(cardTitleStyle.Render(title) + "\n" + strings.Join(rows, "\n"))
}

func truncateText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	return ansi.Truncate(value, limit, "…")
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
			line = focusStyle.Render(line)
		}
		rows[index] = line
	}
	width := model.ui.columnWidth(2)
	if model.ui.compact() {
		width = model.ui.innerWidth()
	}
	return model.ui.innerStyle(width).Render(cardTitleStyle.Render(model.ui.t("screen.tokenUsage")) + "\n" + strings.Join(rows, "\n"))
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
		rows = append(rows, model.ui.t("stats.noRequests"))
	}
	width := model.ui.columnWidth(2)
	if model.ui.compact() {
		width = model.ui.innerWidth()
	}
	return model.ui.innerStyle(width).Render(cardTitleStyle.Render(model.ui.t("screen.recentRequests")) + "\n" + strings.Join(rows, "\n"))
}

func (model *statisticsModel) refresh() {
	defer func() { model.loading = false }()
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
	return map[string]string{"today": "period.today", "24h": "period.24h", "7d": "period.7d", "30d": "period.30d", "60d": "period.60d"}[period]
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
