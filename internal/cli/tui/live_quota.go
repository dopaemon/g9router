package tui

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type quotaRefreshMsg struct{}

type quotaModel struct {
	ui           *UI
	items        []quotaItem
	cursor       int
	usageEnabled bool
	loading      bool
	err          error
	detail       int
	detailCursor int
	itemsTop     int
	itemsLeft    int
	itemsWidth   int
	itemsHeight  int
}

type quotaItem struct {
	ID           string
	Name         string
	Email        string
	Requests     int64
	Errors       int64
	InputTokens  int64
	OutputTokens int64
	Message      string
	Plan         string
	Quotas       map[string]quotaWindow
}

type quotaWindow struct {
	Used      float64 `json:"used"`
	Total     float64 `json:"total"`
	Remaining float64 `json:"remaining"`
	ResetAt   string  `json:"resetAt"`
}

func (ui *UI) liveQuota() error {
	EnableColors(ui.Out)
	return ui.runTea(&quotaModel{ui: ui, loading: true, usageEnabled: true, detail: -1})
}

func (model *quotaModel) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return quotaRefreshMsg{} }, quotaRefresh())
}

func (model *quotaModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if model.detail >= 0 {
			switch message.String() {
			case "q", "esc":
				model.detail = -1
			case "up", "k":
				model.detailCursor = moveIndex(model.detailCursor, 3, -1)
			case "down", "j":
				model.detailCursor = moveIndex(model.detailCursor, 3, 1)
			case "enter", " ":
				switch model.detailCursor {
				case 0:
					model.loading = true
					return model, quotaRefresh()
				case 1:
					if err := model.toggleUsage(); err != nil {
						model.err = err
						return model, nil
					}
					model.loading = true
					return model, quotaRefresh()
				case 2:
					model.detail = -1
				}
			case "r":
				model.loading = true
				return model, quotaRefresh()
			case "u":
				if err := model.toggleUsage(); err != nil {
					model.err = err
					return model, nil
				}
				model.loading = true
				return model, quotaRefresh()
			}
			return model, nil
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			if model.cursor > 0 {
				model.cursor--
			}
		case "down", "j":
			if model.cursor+1 < len(model.items) {
				model.cursor++
			}
		case "enter", " ":
			if len(model.items) > 0 {
				model.detail = model.cursor
			}
		case "r":
			model.loading = true
			return model, quotaRefresh()
		}
	case quotaRefreshMsg:
		model.refresh()
		return model, quotaRefresh()
	}
	return model, nil
}

func (model *quotaModel) toggleUsage() error {
	model.usageEnabled = !model.usageEnabled
	return model.ui.request(http.MethodPut, "/api/settings", map[string]bool{"usageEnabled": model.usageEnabled}, nil)
}

func (model *quotaModel) View() string {
	if model.loading {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n\n" + mutedStyle.Render(model.ui.t("common.loading")))
	}
	if model.err != nil {
		return model.ui.errorView(model.ui.t("menu.quota"), model.err)
	}
	if model.detail >= 0 && model.detail < len(model.items) {
		return model.detailView(model.items[model.detail])
	}
	rows := make([]string, 0, len(model.items))
	for index, item := range model.items {
		label := item.Name + " (" + item.ID + ")"
		if item.Message != "" {
			rows = append(rows, providerMenuItem(index, label+" — "+item.Message, index == model.cursor))
			continue
		}
		if item.Plan != "" {
			label += " · " + item.Plan
		}
		rows = append(rows, providerMenuItem(index, label, index == model.cursor))
		if len(item.Quotas) == 0 {
			rows = append(rows, "   "+model.ui.t("quota.requests")+": "+formatInt(item.Requests)+"  "+model.ui.t("quota.errors")+": "+formatInt(item.Errors))
			continue
		}
		for name, quota := range item.Quotas {
			rows = append(rows, "   "+quotaBar(name, quota, model.ui.innerWidth()))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, mutedStyle.Render(model.ui.t("quota.noProviders")))
	}
	content := cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n\n" + model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota"))+"\n"+strings.Join(rows, "\n"))
	controls := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n↑↓/jk move  Enter select  r refresh  q back")
	return model.ui.outerStyle().Render(content + "\n\n" + controls)
}

func (model *quotaModel) detailView(item quotaItem) string {
	info := []string{
		"Provider: " + item.Name,
		"ID: " + item.ID,
		"Email: " + valueOrDash(item.Email),
		"Plan: " + valueOrDash(item.Plan),
		"Usage: " + model.ui.t(map[bool]string{true: "quota.usageOn", false: "quota.usageOff"}[model.usageEnabled]),
		model.ui.t("quota.requests") + ": " + formatInt(item.Requests),
		model.ui.t("quota.errors") + ": " + formatInt(item.Errors),
		model.ui.t("quota.input") + ": " + formatInt(item.InputTokens),
		model.ui.t("quota.output") + ": " + formatInt(item.OutputTokens),
	}
	if item.Message != "" {
		info = append(info, errorStyle.Render(item.Message))
	}
	infoCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("quota.info")) + "\n" + strings.Join(info, "\n"))
	quotaRows := []string{}
	if len(item.Quotas) == 0 {
		quotaRows = append(quotaRows, model.ui.t("quota.requests")+": "+formatInt(item.Requests), model.ui.t("quota.errors")+": "+formatInt(item.Errors))
		quotaRows = append(quotaRows, model.ui.t("quota.input")+": "+formatInt(item.InputTokens), model.ui.t("quota.output")+": "+formatInt(item.OutputTokens))
	}
	for name, quota := range item.Quotas {
		quotaRows = append(quotaRows, quotaBar(name, quota, model.ui.innerWidth()))
	}
	quotaCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n" + strings.Join(quotaRows, "\n"))
	usageStatus := model.ui.t("quota.usageOff")
	if model.usageEnabled {
		usageStatus = model.ui.t("quota.usageOn")
	}
	actions := []string{model.ui.t("common.refresh"), model.ui.t("quota.toggleUsage") + " (" + usageStatus + ")", model.ui.t("common.back")}
	menuRows := make([]string, 0, len(actions))
	for index, action := range actions {
		menuRows = append(menuRows, providerMenuItem(index, action, index == model.detailCursor))
	}
	menuCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("quota.functions")) + "\n" + strings.Join(menuRows, "\n") + "\n\n" + mutedStyle.Render("↑↓/jk move  Enter select"))
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n\n" + infoCard + "\n\n" + quotaCard + "\n\n" + menuCard)
}

func quotaProviderEmail(item provider) string {
	if len(item.Accounts) > 0 && item.Accounts[0].Email != "" {
		return item.Accounts[0].Email
	}
	for _, key := range []string{"email", "userEmail", "githubEmail"} {
		if email, ok := item.ProviderSpecificData[key].(string); ok && email != "" {
			return email
		}
	}
	return ""
}

func (model *quotaModel) refresh() {
	defer func() { model.loading = false }()
	var settings map[string]any
	if err := model.ui.request(http.MethodGet, "/api/settings", nil, &settings); err == nil {
		if _, ok := settings["usageEnabled"]; ok {
			model.usageEnabled = settingsEnabled(settings, "usageEnabled")
		}
	}
	var response providersResponse
	if err := model.ui.request(http.MethodGet, "/api/providers", nil, &response); err != nil {
		model.err = err
		return
	}
	items := make([]quotaItem, 0, len(response.Connections))
	for _, provider := range response.Connections {
		item := quotaItem{ID: provider.ID, Name: provider.Name, Email: quotaProviderEmail(provider)}
		if item.Name == "" {
			item.Name = provider.ID
		}
		var usage struct {
			Requests     int64                  `json:"requests"`
			Errors       int64                  `json:"errors"`
			InputTokens  int64                  `json:"inputTokens"`
			OutputTokens int64                  `json:"outputTokens"`
			Message      string                 `json:"message"`
			Plan         string                 `json:"plan"`
			Quotas       map[string]quotaWindow `json:"quotas"`
		}
		if !model.usageEnabled {
			item.Message = model.ui.t("quota.usageOff")
			items = append(items, item)
			continue
		}
		if err := model.ui.request(http.MethodGet, "/api/usage/"+url.PathEscape(provider.ID), nil, &usage); err != nil {
			usage.Message = model.ui.t("quota.unavailable")
		}
		item.Requests, item.Errors = usage.Requests, usage.Errors
		item.InputTokens, item.OutputTokens = usage.InputTokens, usage.OutputTokens
		item.Message, item.Plan, item.Quotas = usage.Message, usage.Plan, usage.Quotas
		items = append(items, item)
	}
	model.items, model.err = items, nil
	if model.cursor >= len(model.items) {
		model.cursor = 0
	}
}

func quotaBar(name string, quota quotaWindow, width int) string {
	remaining := quota.Remaining
	if quota.Total > 0 && remaining == 0 && quota.Used < quota.Total {
		remaining = quota.Total - quota.Used
	}
	if quota.Total > 0 && remaining > 1 {
		remaining = remaining / quota.Total * 100
	}
	remaining = math.Max(0, math.Min(100, remaining))
	barWidth := 20
	if width < 60 {
		barWidth = 12
	}
	filled := int(math.Round(remaining / 100 * float64(barWidth)))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	style := errorStyle
	if remaining >= 70 {
		style = successStyle
	} else if remaining >= 30 {
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FACC15"))
	}
	reset := ""
	if quota.ResetAt != "" {
		if resetAt, err := time.Parse(time.RFC3339, quota.ResetAt); err == nil {
			reset = "  · reset " + formatDuration(time.Until(resetAt))
		}
	}
	return fmt.Sprintf("%-8s %s %3.0f%%%s", name, style.Render("["+bar+"]"), remaining, mutedStyle.Render(reset))
}

func formatDuration(duration time.Duration) string {
	if duration <= 0 {
		return "now"
	}
	duration = duration.Round(time.Minute)
	if duration < time.Minute {
		return "<1m"
	}
	days := duration / (24 * time.Hour)
	hours := duration % (24 * time.Hour) / time.Hour
	minutes := duration % time.Hour / time.Minute
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func quotaRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return quotaRefreshMsg{} })
}
