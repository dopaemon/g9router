package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"g9router/internal/providers"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type providerRefreshMsg struct{}
type providerActionDoneMsg struct {
	notice string
	err    error
}

type providerTab int

const (
	customProviderTab providerTab = iota
	oauthProviderTab
	freeProviderTab
	apiKeyProviderTab
)

type oauthProvider struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

type providerLiveModel struct {
	ui               *UI
	tab              providerTab
	cursor           int
	custom           []provider
	oauth            []oauthProvider
	oauthConnections []provider
	free             []provider
	apiKeys          []provider
	notice           string
	err              error
	loading          bool
	tabsTop          int
	itemsTop         int
	itemsLeft        int
	itemsWidth       int
	itemsStart       int
	itemsHeight      int
	tabRegions       []tuiRegion
}

var freeProviderNames = []string{"mimo-free"}

func (ui *UI) liveProviders() error {
	EnableColors(ui.Out)
	model := providerLiveModel{ui: ui, loading: true}
	return ui.runTea(&model)
}

func (model *providerLiveModel) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg { return providerRefreshMsg{} }, providerRefresh())
}

func (model *providerLiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.MouseMsg:
		if tab, ok := model.providerMouseTab(message.X, message.Y); ok && (message.Action == tea.MouseActionPress || message.Action == tea.MouseActionRelease) && message.Button == tea.MouseButtonLeft {
			model.tab, model.cursor = tab, 0
			return model, nil
		}
		if index := model.providerMouseIndex(message.X, message.Y); index >= 0 && index < model.itemCount() {
			model.cursor = index
			if (message.Action == tea.MouseActionPress || message.Action == tea.MouseActionRelease) && message.Button == tea.MouseButtonLeft {
				return model, model.runAction()
			}
		}
	case tea.KeyMsg:
		if model.err != nil && message.String() == "r" {
			model.err = nil
			model.refresh()
			return model, providerRefresh()
		}
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= model.itemCount() {
			model.cursor = index - 1
			return model, model.runAction()
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "tab", "right", "l":
			model.tab = providerTab(cycleIndex(int(model.tab), 4, 1))
			model.cursor = 0
		case "shift+tab", "left", "h":
			model.tab = providerTab(cycleIndex(int(model.tab), 4, -1))
			model.cursor = 0
		case "up", "k":
			if model.cursor > 0 {
				model.cursor--
			}
		case "down", "j":
			if model.cursor+1 < model.itemCount() {
				model.cursor++
			}
		case "enter", " ":
			return model, model.runAction()
		case "e":
			return model, model.action(model.edit)
		case "d":
			return model, model.action(model.delete)
		case "a":
			return model, model.action(model.add)
		}
	case providerRefreshMsg:
		model.refresh()
		return model, providerRefresh()
	case providerActionDoneMsg:
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.refresh()
		}
		return model, providerRefresh()
	}
	return model, nil
}

func (model *providerLiveModel) providerMouseTab(x, y int) (providerTab, bool) {
	for index, region := range model.tabRegions {
		if region.contains(x, y) {
			return providerTab(index), true
		}
	}
	return 0, false
}

func (model *providerLiveModel) providerMouseIndex(x, y int) int {
	if x < model.itemsLeft || x >= model.itemsLeft+model.itemsWidth || y < model.itemsTop || model.itemsHeight > 0 && y >= model.itemsTop+model.itemsHeight {
		return -1
	}
	return model.itemsStart + y - model.itemsTop
}

func (model *providerLiveModel) View() string {
	model.tabRegions = nil
	tabs := []string{model.ui.t("tab.custom"), model.ui.t("tab.oauth"), model.ui.t("tab.free"), model.ui.t("tab.apiKey")}
	tabLine := make([]string, len(tabs))
	for index, tab := range tabs {
		style := mutedStyle.Padding(0, 1)
		if providerTab(index) == model.tab {
			style = focusStyle
		}
		tabLine[index] = style.Render(tab)
	}
	content := model.cardContent()
	column := model.ui.controlStyle()
	actions := model.ui.t("controls.providerEditDelete")
	if model.tab == customProviderTab {
		actions = model.ui.t("controls.providerCustomActions")
	}
	controlRows := []string{
		column.Render(mutedStyle.Render(model.ui.t("controls.moveSwitch"))),
		column.Render(mutedStyle.Render(model.ui.t("controls.selectEdit"))),
		column.Render(mutedStyle.Render(actions)),
		column.Render(mutedStyle.Render(model.ui.t("controls.deleteBack"))),
	}
	controls := strings.Join(controlRows, "\n")
	if hint := model.ui.mouseHint(); hint != "" {
		controls += "\n" + mutedStyle.Render(hint)
	}
	if model.notice != "" {
		controls += "\n" + successStyle.Render(model.notice)
	}
	if model.err != nil {
		controls += "\n" + errorStyle.Render("ERROR: "+model.ui.errorSummary(model.err))
	}
	controlCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + controls)
	model.tabsTop = 2 + lipgloss.Height(cardTitleStyle.Render(model.ui.t("menu.providers"))) + 2
	tabView := lipgloss.JoinHorizontal(lipgloss.Top, tabLine...)
	if model.ui.compact() {
		tabView = lipgloss.JoinVertical(lipgloss.Left, tabLine...)
	}
	if model.ui.compact() {
		for index := range tabLine {
			model.tabRegions = append(model.tabRegions, tuiRegion{left: 1 + 2, top: model.tabsTop + index, width: model.ui.innerWidth(), height: 1})
		}
	} else {
		left := 1 + 2
		for index, tab := range tabLine {
			width := lipgloss.Width(tab)
			model.tabRegions = append(model.tabRegions, tuiRegion{left: left, top: model.tabsTop, width: width, height: 1})
			left += width
			if index+1 < len(tabLine) {
				left += 2
			}
		}
	}
	model.itemsTop = model.tabsTop + lipgloss.Height(tabView) + 2 + 2 + 1
	model.itemsLeft = 1 + 2 + 1 + 2
	model.itemsWidth = model.ui.innerWidth()
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.providers")) + "\n\n" + tabView + "\n\n" + content + "\n\n" + controlCard)
}

func (model *providerLiveModel) cardContent() string {
	if model.loading {
		return model.ui.innerStyle().Render(mutedStyle.Render(model.ui.t("common.loading")))
	}
	var title string
	var rows []string
	switch model.tab {
	case customProviderTab:
		title = model.ui.t("screen.customProviders")
		rows = []string{model.ui.t("provider.addAnthropic"), model.ui.t("provider.addOpenAI")}
		for _, item := range model.custom {
			rows = append(rows, item.Name+" ("+item.ID+")")
		}
	case oauthProviderTab:
		title = model.ui.t("screen.oauthProviders")
		rows = []string{model.ui.t("provider.addOAuth")}
		for _, item := range model.oauthConnections {
			name := item.Name
			if name == "" {
				name = item.ID
			}
			rows = append(rows, name+" ("+item.ID+") ["+statusTextLocale(item.Enabled, model.ui.Locale)+"]")
		}
	case freeProviderTab:
		title = model.ui.t("screen.freeProviders")
		for _, name := range freeProviderNames {
			item := model.find(model.free, name)
			rows = append(rows, name+" ["+statusTextLocale(item != nil && item.Enabled, model.ui.Locale)+"]")
		}
	case apiKeyProviderTab:
		title = model.ui.t("screen.apiKeyProviders")
		for _, item := range model.apiKeys {
			rows = append(rows, item.Name+" ("+item.ID+") ["+statusTextLocale(item.Enabled, model.ui.Locale)+"]")
		}
	}
	if len(rows) == 0 {
		rows = []string{model.ui.t("screen.noProviders")}
	}
	start, end := viewportWindow(model.cursor, len(rows), model.ui.viewportHeight(14, 10))
	model.itemsStart, model.itemsHeight = start, end-start
	items := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		items = append(items, providerMenuItem(index, rows[index], index == model.cursor))
	}
	return model.ui.innerStyle().Render(cardTitleStyle.Render(title) + "\n" + strings.Join(items, "\n"))
}

func providerMenuItem(index int, label string, selected bool) string {
	text := string(rune('1'+index)) + "  " + label
	if selected {
		return focusStyle.Render(text)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Padding(0, 1).Render(text)
}

func (model *providerLiveModel) itemCount() int {
	switch model.tab {
	case customProviderTab:
		return 2 + len(model.custom)
	case oauthProviderTab:
		return 1 + len(model.oauthConnections)
	case freeProviderTab:
		return len(freeProviderNames)
	default:
		return len(model.apiKeys)
	}
}

func (model *providerLiveModel) runAction() tea.Cmd {
	if model.tab == customProviderTab && model.cursor < 2 {
		return model.action(model.add)
	}
	if model.tab == oauthProviderTab && model.cursor == 0 {
		return model.action(model.addOAuth)
	}
	return model.action(model.edit)
}

func (model *providerLiveModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
	var notice string
	return tea.Exec(&endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		var err error
		notice, err = run(input, output)
		return err
	}}, func(err error) tea.Msg { return providerActionDoneMsg{notice: notice, err: err} })
}

func (model *providerLiveModel) add(input io.Reader, output io.Writer) (string, error) {
	if model.tab != customProviderTab || model.cursor > 1 {
		return "", nil
	}
	apiType := "anthropic"
	if model.cursor == 1 {
		apiType = "openai"
	}
	item := &provider{Enabled: true, APIType: apiType}
	if descriptor, ok := providers.Lookup(apiType); ok {
		item.ID, item.Name, item.BaseURL = apiType, apiType, descriptor.BaseURL
	}
	if err := model.ui.promptProviderTUI(item, false, input, output); err != nil {
		return "", err
	}
	return model.ui.t("notice.providerAdded"), nil
}

func (model *providerLiveModel) addOAuth(input io.Reader, output io.Writer) (string, error) {
	if model.tab != oauthProviderTab || model.cursor != 0 {
		return "", nil
	}
	oldIn, oldOut := model.ui.In, model.ui.Out
	model.ui.In, model.ui.Out = input, output
	err := model.ui.loginOAuthProvider(bufio.NewReader(input))
	model.ui.In, model.ui.Out = oldIn, oldOut
	return model.ui.t("notice.oauthAdded"), err
}

func (model *providerLiveModel) edit(input io.Reader, output io.Writer) (string, error) {
	if model.tab == customProviderTab {
		index := model.cursor - 2
		if index < 0 || index >= len(model.custom) {
			return "", nil
		}
		return model.editProvider(model.custom[index], input, output)
	}
	if model.tab == apiKeyProviderTab {
		if model.cursor >= len(model.apiKeys) {
			return "", nil
		}
		return model.editProvider(model.apiKeys[model.cursor], input, output)
	}
	if model.tab == oauthProviderTab {
		index := model.cursor - 1
		if index < 0 || index >= len(model.oauthConnections) {
			return "", nil
		}
		item := model.oauthConnections[index]
		return model.ui.t("notice.providerUpdated"), model.ui.request(http.MethodPut, "/api/providers/"+url.PathEscape(item.ID), map[string]bool{"enabled": !item.Enabled}, nil)
	}
	return "", nil
}

func (model *providerLiveModel) editProvider(item provider, input io.Reader, output io.Writer) (string, error) {
	var current struct {
		Connection provider `json:"connection"`
	}
	if err := model.ui.request(http.MethodGet, "/api/providers/"+url.PathEscape(item.ID), nil, &current); err != nil {
		return "", err
	}
	return model.ui.t("notice.providerUpdated"), model.ui.promptProviderTUI(&current.Connection, true, input, output)
}

func (model *providerLiveModel) delete(input io.Reader, output io.Writer) (string, error) {
	if model.tab == customProviderTab {
		index := model.cursor - 2
		if index < 0 || index >= len(model.custom) {
			return "", nil
		}
		item := model.custom[index]
		return model.ui.t("notice.providerDeleted"), model.deleteProvider(item.ID, input, output)
	}
	if model.tab == apiKeyProviderTab && model.cursor < len(model.apiKeys) {
		return model.ui.t("notice.providerDeleted"), model.deleteProvider(model.apiKeys[model.cursor].ID, input, output)
	}
	if model.tab == oauthProviderTab {
		index := model.cursor - 1
		if index < 0 || index >= len(model.oauthConnections) {
			return "", nil
		}
		return model.ui.t("notice.oauthDeleted"), model.deleteOAuthProvider(model.oauthConnections[index], input, output)
	}
	return "", nil
}

func (model *providerLiveModel) deleteProvider(id string, input io.Reader, output io.Writer) error {
	ok, err := model.ui.tuiConfirm(fmt.Sprintf(model.ui.t("confirm.deleteProvider"), id), input, output)
	if err != nil || !ok {
		return err
	}
	return model.ui.request(http.MethodDelete, "/api/providers?id="+url.QueryEscape(id), nil, nil)
}

func (model *providerLiveModel) deleteOAuthProvider(item provider, input io.Reader, output io.Writer) error {
	ok, err := model.ui.tuiConfirm(model.ui.t("confirm.deleteOAuthProvider"), input, output)
	if err != nil || !ok {
		return err
	}
	if err := model.ui.request(http.MethodDelete, "/api/providers?id="+url.QueryEscape(item.ID), nil, nil); err != nil {
		return err
	}
	if credential := model.oauthCredential(item); credential != nil {
		return model.ui.request(http.MethodDelete, "/api/oauth/"+url.PathEscape(credential.ID), nil, nil)
	}
	return nil
}

func (model *providerLiveModel) loginOAuth(input io.Reader, output io.Writer, name string) error {
	oldIn, oldOut := model.ui.In, model.ui.Out
	model.ui.In, model.ui.Out = input, output
	err := model.ui.loginBrowserOAuth(bufio.NewReader(input), name)
	model.ui.In, model.ui.Out = oldIn, oldOut
	return err
}

func (model *providerLiveModel) refresh() {
	defer func() { model.loading = false }()
	var response providersResponse
	if err := model.ui.request(http.MethodGet, "/api/providers", nil, &response); err != nil {
		model.err = err
		return
	}
	var credentials []oauthProvider
	if err := model.ui.request(http.MethodGet, "/api/oauth", nil, &credentials); err != nil {
		model.err = err
		return
	}
	model.custom, model.oauthConnections, model.apiKeys = nil, nil, nil
	for _, item := range response.Connections {
		if item.OAuthID != "" {
			model.oauthConnections = append(model.oauthConnections, item)
			continue
		}
		if item.APIType == "openai" || item.APIType == "anthropic" {
			model.custom = append(model.custom, item)
		} else {
			model.apiKeys = append(model.apiKeys, item)
		}
	}
	model.free = nil
	for _, name := range freeProviderNames {
		for _, item := range response.Connections {
			if item.ID == name {
				model.free = append(model.free, item)
			}
		}
	}
	model.oauth = credentials
	model.err = nil
	if model.cursor >= model.itemCount() {
		model.cursor = 0
	}
}

func (model *providerLiveModel) oauthCredential(item provider) *oauthProvider {
	for index := range model.oauth {
		if model.oauth[index].ID == item.OAuthID || model.oauth[index].Provider == item.ID {
			return &model.oauth[index]
		}
	}
	return nil
}

func (model *providerLiveModel) find(items []provider, id string) *provider {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}

func providerRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return providerRefreshMsg{} })
}
