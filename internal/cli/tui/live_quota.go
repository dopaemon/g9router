package tui

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type quotaRefreshMsg struct{}
type quotaActionDoneMsg struct{ err error }
type quotaDataMsg struct {
	items        []quotaItem
	usageEnabled bool
	err          error
}

type quotaModel struct {
	ui           *UI
	items        []quotaItem
	cursor       int
	usageEnabled bool
	autoRefresh  bool
	loading      bool
	refreshing   bool
	err          error
	detail       int
	detailCursor int
	itemsTop     int
	itemsLeft    int
	itemsWidth   int
	itemsHeight  int
}

type quotaItem struct {
	ID                string
	Name              string
	Email             string
	Requests          int64
	Errors            int64
	InputTokens       int64
	OutputTokens      int64
	Message           string
	Plan              string
	Quotas            map[string]quotaWindow
	ResetCredits      int
	ResetCreditsKnown bool
}

type quotaWindow struct {
	Used      float64 `json:"used"`
	Total     float64 `json:"total"`
	Remaining float64 `json:"remaining"`
	ResetAt   string  `json:"resetAt"`
}

func (ui *UI) liveQuota() error {
	EnableColors(ui.Out)
	return ui.runTea(&quotaModel{ui: ui, loading: true, usageEnabled: true, autoRefresh: true, detail: -1})
}

func (model *quotaModel) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return quotaRefreshMsg{} }, quotaRefresh())
}

func (model *quotaModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case quotaDataMsg:
		if message.err == nil {
			model.items = message.items
		}
		model.usageEnabled = message.usageEnabled
		model.err = message.err
		model.loading = false
		model.refreshing = false
		if model.cursor >= len(model.items) {
			model.cursor = 0
		}
		if model.detail >= len(model.items) {
			model.detail = -1
			model.detailCursor = 0
		}
		return model, nil
	case quotaActionDoneMsg:
		model.refreshing = false
		if errors.Is(message.err, huh.ErrUserAborted) {
			model.err = nil
			return model, nil
		}
		model.err = message.err
		if model.err == nil {
			model.refreshing = true
			return model, model.refreshCmd()
		}
		return model, nil
	case tea.KeyMsg:
		if model.detail >= 0 {
			if model.detail >= len(model.items) {
				model.detail = -1
				return model, nil
			}
			item := model.items[model.detail]
			switch message.String() {
			case "q", "esc":
				model.detail = -1
			case "up", "k":
				model.detailCursor = moveIndex(model.detailCursor, model.detailActionCount(item), -1)
			case "down", "j":
				model.detailCursor = moveIndex(model.detailCursor, model.detailActionCount(item), 1)
			case "enter", " ":
				if model.refreshing {
					return model, nil
				}
				switch model.detailCursor {
				case 0:
					model.refreshing = true
					return model, model.refreshCmd()
				case 1:
					if err := model.toggleUsage(); err != nil {
						model.err = err
						return model, nil
					}
					model.refreshing = true
					return model, model.refreshCmd()
				case 2:
					if item.ID == "codex" {
						if item.ResetCreditsKnown && item.ResetCredits > 0 {
							model.refreshing = true
							return model, model.resetCodexLimit(item)
						}
						return model, nil
					}
					model.detail = -1
				case 3:
					model.detail = -1
				}
			case "r":
				if model.refreshing {
					return model, nil
				}
				model.refreshing = true
				return model, model.refreshCmd()
			case "u":
				if model.refreshing {
					return model, nil
				}
				if err := model.toggleUsage(); err != nil {
					model.err = err
					return model, nil
				}
				model.refreshing = true
				return model, model.refreshCmd()
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
			if !model.refreshing && len(model.items) > 0 {
				model.detail = model.cursor
			}
		case "r":
			if model.refreshing {
				return model, nil
			}
			model.refreshing = true
			return model, model.refreshCmd()
		case "a":
			model.autoRefresh = !model.autoRefresh
			if model.autoRefresh {
				model.refreshing = true
				return model, tea.Batch(model.refreshCmd(), quotaRefresh())
			}
		}
	case quotaRefreshMsg:
		if !model.autoRefresh {
			return model, nil
		}
		if model.refreshing {
			return model, quotaRefresh()
		}
		model.refreshing = true
		return model, tea.Batch(model.refreshCmd(), quotaRefresh())
	}
	return model, nil
}

func (model *quotaModel) detailActionCount(item quotaItem) int {
	if item.ID == "codex" {
		return 4
	}
	return 3
}

func (model *quotaModel) resetCodexLimit(item quotaItem) tea.Cmd {
	return tea.Exec(&endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		ok, err := model.ui.tuiConfirm("Reset Codex limit?", input, output)
		if err != nil || !ok {
			return err
		}
		return model.ui.request(http.MethodPost, "/api/usage/"+url.PathEscape(item.ID)+"/codex-reset-credits", nil, nil)
	}}, func(err error) tea.Msg { return quotaActionDoneMsg{err: err} })
}

func (model *quotaModel) toggleUsage() error {
	model.usageEnabled = !model.usageEnabled
	return model.ui.request(http.MethodPut, "/api/settings", map[string]bool{"usageEnabled": model.usageEnabled}, nil)
}

func (model *quotaModel) View() string {
	if model.loading {
		return model.loadingView()
	}
	if model.err != nil && len(model.items) == 0 {
		return model.ui.errorView(model.ui.t("menu.quota"), model.err)
	}
	if model.detail >= 0 && model.detail < len(model.items) {
		return model.detailView(model.items[model.detail])
	}
	visible := max(1, min(3, model.ui.viewportHeight(14, 10)/4))
	start, end := viewportWindow(model.cursor, len(model.items), visible)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		item := model.items[index]
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
		for _, name := range sortedQuotaNames(item.Quotas) {
			quota := item.Quotas[name]
			rows = append(rows, "   "+quotaBar(name, quota, model.ui.innerWidth()))
		}
	}
	if len(rows) > 10 {
		rows = rows[:10]
	}
	if len(rows) == 0 {
		rows = append(rows, mutedStyle.Render(model.ui.t("quota.noProviders")))
	}
	content := cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n\n" + model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota"))+"\n"+strings.Join(rows, "\n"))
	autoRefresh := "OFF"
	if model.autoRefresh {
		autoRefresh = "ON"
	}
	controls := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumns(
		"↑↓/jk move", "Enter select", "r refresh", "a auto-refresh ("+autoRefresh+")", "q back",
	))
	return model.ui.outerStyle().Render(content + "\n\n" + controls)
}

func (model *quotaModel) loadingView() string {
	line := mutedStyle.Render(truncateText("░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░", max(1, model.ui.innerWidth()-4)))
	rows := strings.Join([]string{line, line, line}, "\n")
	content := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n" + rows)
	controls := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumns(
		"↑↓/jk move", "Enter select", "r refresh", "a auto-refresh", "q back",
	))
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n\n" + content + "\n\n" + controls)
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
	if item.ID == "codex" {
		credits := "-"
		if item.ResetCreditsKnown {
			credits = formatInt(int64(item.ResetCredits))
		}
		info = append(info, model.ui.t("quota.resetCredits")+": "+credits)
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
	for _, name := range sortedQuotaNames(item.Quotas) {
		quota := item.Quotas[name]
		quotaRows = append(quotaRows, quotaBar(name, quota, model.ui.innerWidth()))
	}
	if len(quotaRows) > 10 {
		quotaRows = quotaRows[:10]
	}
	quotaCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n" + strings.Join(quotaRows, "\n"))
	usageStatus := model.ui.t("quota.usageOff")
	if model.usageEnabled {
		usageStatus = model.ui.t("quota.usageOn")
	}
	actions := []string{model.ui.t("common.refresh"), model.ui.t("quota.toggleUsage") + " (" + usageStatus + ")"}
	if item.ID == "codex" {
		actions = append(actions, model.ui.t("quota.resetLimit"))
	}
	actions = append(actions, model.ui.t("common.back"))
	controlItems := make([]string, 0, len(actions))
	for index, action := range actions {
		disabled := item.ID == "codex" && index == 2 && (!item.ResetCreditsKnown || item.ResetCredits == 0)
		if disabled {
			controlItems = append(controlItems, action+" (disabled)")
			continue
		}
		controlItems = append(controlItems, action)
	}
	menuCard := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumnsSelected(model.detailCursor, controlItems...))
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.quota")) + "\n\n" + infoCard + "\n\n" + quotaCard + "\n\n" + menuCard)
}

func sortedQuotaNames(quotas map[string]quotaWindow) []string {
	names := make([]string, 0, len(quotas))
	for name := range quotas {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

func (model *quotaModel) refreshCmd() tea.Cmd {
	ui, usageEnabled := model.ui, model.usageEnabled
	return func() tea.Msg { return loadQuota(ui, usageEnabled) }
}

func loadQuota(ui *UI, usageEnabled bool) quotaDataMsg {
	var settings map[string]any
	if err := ui.request(http.MethodGet, "/api/settings", nil, &settings); err == nil {
		if _, ok := settings["usageEnabled"]; ok {
			usageEnabled = settingsEnabled(settings, "usageEnabled")
		}
	}
	var response providersResponse
	if err := ui.request(http.MethodGet, "/api/providers", nil, &response); err != nil {
		return quotaDataMsg{usageEnabled: usageEnabled, err: err}
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
			ResetCredits *struct {
				AvailableCount int `json:"availableCount"`
			} `json:"resetCredits"`
		}
		if !usageEnabled {
			item.Message = ui.t("quota.usageOff")
			items = append(items, item)
			continue
		}
		if err := ui.request(http.MethodGet, "/api/usage/"+url.PathEscape(provider.ID), nil, &usage); err != nil {
			usage.Message = ui.t("quota.unavailable")
		}
		item.Requests, item.Errors = usage.Requests, usage.Errors
		item.InputTokens, item.OutputTokens = usage.InputTokens, usage.OutputTokens
		item.Message, item.Plan, item.Quotas = usage.Message, usage.Plan, usage.Quotas
		if usage.ResetCredits != nil {
			item.ResetCredits = usage.ResetCredits.AvailableCount
			item.ResetCreditsKnown = true
		}
		items = append(items, item)
	}
	return quotaDataMsg{items: items, usageEnabled: usageEnabled}
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
