package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"g9router/internal/providers"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type providerRefreshMsg struct{}
type providerNumberMsg struct{ value string }
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
	refreshing       bool
	testing          bool
	lastAction       string
	numberInput      string
	tabsTop          int
	itemsTop         int
	itemsLeft        int
	itemsWidth       int
	itemsStart       int
	itemsHeight      int
	tabRegions       []tuiRegion
}

type providerDataMsg struct {
	custom           []provider
	oauth            []oauthProvider
	oauthConnections []provider
	free             []provider
	apiKeys          []provider
	err              error
}

var freeProviderNames = []string{"mimo-free"}

func (ui *UI) liveProviders() error {
	EnableColors(ui.Out)
	model := providerLiveModel{ui: ui, loading: true}
	return ui.runTea(&model)
}

func (model *providerLiveModel) Init() tea.Cmd {
	model.refreshing = true
	return model.refreshCmd()
}

func (model *providerLiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case providerDataMsg:
		if message.err == nil {
			model.custom, model.oauth, model.oauthConnections = message.custom, message.oauth, message.oauthConnections
			model.free, model.apiKeys = message.free, message.apiKeys
		}
		model.err, model.loading, model.refreshing = message.err, false, false
		if model.cursor >= model.itemCount() {
			model.cursor = max(0, model.itemCount()-1)
		}
		return model, providerRefresh()
	case providerNumberMsg:
		if message.value != model.numberInput {
			return model, nil
		}
		model.numberInput = ""
		index, err := strconv.Atoi(message.value)
		if err != nil || index < 1 || index > model.itemCount() || model.loading || model.testing {
			return model, nil
		}
		model.cursor = index - 1
		return model, model.runAction()
	case tea.MouseMsg:
		if model.testing {
			return model, nil
		}
		if tab, ok := model.providerMouseTab(message.X, message.Y); ok && (message.Action == tea.MouseActionPress || message.Action == tea.MouseActionRelease) && message.Button == tea.MouseButtonLeft {
			model.tab, model.cursor = tab, 0
			return model, nil
		}
		if model.loading {
			return model, nil
		}
		if index := model.providerMouseIndex(message.X, message.Y); index >= 0 && index < model.itemCount() {
			model.cursor = index
			if message.Action == tea.MouseActionPress && message.Button == tea.MouseButtonLeft {
				return model, model.runAction()
			}
		}
	case tea.KeyMsg:
		key := message.String()
		if key == "q" || key == "esc" || key == "ctrl+c" {
			return model, tea.Quit
		}
		if model.testing {
			return model, nil
		}
		if len(key) != 1 || key[0] < '0' || key[0] > '9' {
			model.numberInput = ""
		}
		if model.err != nil && message.String() == "r" {
			if model.lastAction == "test" {
				return model, model.testAction()
			}
			model.err = nil
			model.loading = true
			model.refreshing = true
			return model, model.refreshCmd()
		}
		if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
			if len(model.numberInput) < 2 {
				model.numberInput += key
				value := model.numberInput
				return model, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return providerNumberMsg{value: value} })
			}
			return model, nil
		}
		switch message.String() {
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
			if !model.loading {
				return model, model.runAction()
			}
		case "e":
			if !model.loading && model.canEdit() {
				return model, model.action(model.edit)
			}
		case "t":
			if !model.loading && model.canTest() {
				return model, model.testAction()
			}
		case "d":
			if !model.loading && model.canDelete() {
				return model, model.action(model.delete)
			}
		case "a":
			if !model.loading && model.canAdd() {
				return model, model.action(model.add)
			}
		}
	case providerRefreshMsg:
		if model.refreshing {
			return model, providerRefresh()
		}
		model.refreshing = true
		return model, model.refreshCmd()
	case providerActionDoneMsg:
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
			message.notice = ""
		}
		model.testing = false
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.lastAction = ""
			model.loading = true
			model.refreshing = true
			return model, model.refreshCmd()
		}
		return model, providerRefresh()
	}
	return model, nil
}

func (model *providerLiveModel) providerMouseTab(x, y int) (providerTab, bool) {
	if model.ui != nil && model.ui.viewClipped {
		return 0, false
	}
	for index, region := range model.tabRegions {
		if region.contains(x, y) {
			return providerTab(index), true
		}
	}
	return 0, false
}

func (model *providerLiveModel) providerMouseIndex(x, y int) int {
	if model.ui != nil && model.ui.viewClipped {
		return -1
	}
	if x < model.itemsLeft || x >= model.itemsLeft+model.itemsWidth || y < model.itemsTop || model.itemsHeight > 0 && y >= model.itemsTop+model.itemsHeight {
		return -1
	}
	return model.itemsStart + y - model.itemsTop
}

func (model *providerLiveModel) View() string {
	model.tabRegions = nil
	tabs := []string{model.ui.t("tab.custom"), model.ui.t("tab.oauth"), model.ui.t("tab.free"), model.ui.t("tab.apiKey")}
	tabLine := make([]string, len(tabs))
	for index := range tabs {
		style := mutedStyle.Padding(0, 1)
		label := "  " + tabs[index]
		if providerTab(index) == model.tab {
			style = focusStyle
			label = "› " + tabs[index]
		}
		tabLine[index] = style.Render(label)
	}
	content := model.cardContent()
	controlItems := []string{model.ui.t("controls.providerMove"), model.ui.t("controls.providerSwitch")}
	busy := model.loading
	if !busy {
		if _, ok := model.selectedProvider(); ok {
			controlItems = append(controlItems, model.providerActions()...)
			controlItems = append(controlItems, model.ui.t("controls.providerSelect"))
		} else if model.canAdd() {
			controlItems = append(controlItems, model.ui.t("controls.providerSelect"), model.ui.t("controls.providerAddAction"))
		}
	} else {
		controlItems = append(controlItems, model.ui.t("common.loading"))
	}
	controls := model.ui.controlColumns(controlItems...)
	if hint := model.ui.mouseHint(); hint != "" {
		controls += "\n" + mutedStyle.Render(hint)
	}
	if model.notice != "" {
		controls += "\n" + successStyle.Render(model.notice)
	}
	if model.testing {
		controls += "\n" + mutedStyle.Render(model.ui.t("notice.modelTestRunning"))
	}
	controlCard := model.ui.controlCard(model.ui.t("common.controls"), controls)
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
		for _, tab := range tabLine {
			width := lipgloss.Width(tab)
			model.tabRegions = append(model.tabRegions, tuiRegion{left: left, top: model.tabsTop, width: width, height: 1})
			left += width
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
			label := oauthProviderDisplayName(item)
			if !strings.HasPrefix(item.ID, "codex") {
				label += " (" + item.ID + ")"
			}
			rows = append(rows, label+" ["+statusTextLocale(item.Enabled, model.ui.Locale)+"]")
		}
	case freeProviderTab:
		title = model.ui.t("screen.freeProviders")
		for _, item := range model.free {
			rows = append(rows, item.Name+" ("+item.ID+") ["+statusTextLocale(item.Enabled, model.ui.Locale)+"]")
		}
	case apiKeyProviderTab:
		title = model.ui.t("screen.apiKeyProviders")
		for _, item := range model.apiKeys {
			rows = append(rows, item.Name+" ("+item.ID+") ["+statusTextLocale(item.Enabled, model.ui.Locale)+"]")
		}
	}
	if len(rows) == 0 {
		model.itemsStart, model.itemsHeight = 0, 0
		return model.ui.innerStyle().Render(model.providerContent(title, mutedStyle.Render(model.ui.t("screen.noProviders"))))
	}
	start, end := viewportWindow(model.cursor, len(rows), max(1, min(10, model.ui.viewportHeight(14, 10))))
	model.itemsStart, model.itemsHeight = start, end-start
	items := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		items = append(items, fitProviderMenuItem(index, rows[index], index == model.cursor, model.ui.innerWidth()))
	}
	return model.ui.innerStyle().Render(model.providerContent(title, strings.Join(items, "\n")))
}

func (model *providerLiveModel) providerContent(title, body string) string {
	if model.err != nil {
		body = errorStyle.Render(model.ui.t("common.error")+": "+model.ui.errorSummary(model.err)) + "\n" + mutedStyle.Render(model.ui.t("controls.refresh")) + "\n" + body
	}
	return cardTitleStyle.Render(title) + "\n" + body
}

func fitProviderMenuItem(index int, label string, selected bool, width int) string {
	prefixWidth := lipgloss.Width(providerMenuItem(index, "", selected))
	labelWidth := max(0, width-prefixWidth)
	row := providerMenuItem(index, truncateText(label, labelWidth), selected)
	return truncateText(row, max(0, width))
}

func oauthProviderDisplayName(item provider) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = item.ID
	}
	if len(item.Accounts) > 0 {
		accounts := make([]string, 0, len(item.Accounts))
		for _, account := range item.Accounts {
			label := strings.TrimSpace(account.Name)
			if label == "" {
				label = strings.TrimSpace(account.Email)
			}
			if label == "" {
				label = name
			}
			if strings.HasPrefix(item.ID, "codex") && account.Plan != "" {
				label += " · " + account.Plan
			} else if account.Plan != "" && !strings.Contains(strings.ToLower(label), strings.ToLower(account.Plan)) {
				label += " (" + account.Plan + ")"
			}
			accounts = append(accounts, label)
		}
		if len(accounts) > 0 {
			return strings.Join(accounts, " / ")
		}
	}
	email := ""
	if email == "" {
		for _, key := range []string{"email", "userEmail", "githubEmail"} {
			if value, ok := item.ProviderSpecificData[key].(string); ok && strings.TrimSpace(value) != "" {
				email = strings.TrimSpace(value)
				break
			}
		}
	}
	if email != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(email)) {
		name += " " + email
	}
	return name
}

func providerMenuItem(index int, label string, selected bool) string {
	marker := " "
	if selected {
		marker = "›"
	}
	text := fmt.Sprintf("%s %2d  %s", marker, index+1, label)
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
		return len(model.free)
	default:
		return len(model.apiKeys)
	}
}

func (model *providerLiveModel) canAdd() bool {
	return model.tab == customProviderTab && model.cursor < 2 || model.tab == oauthProviderTab && model.cursor == 0
}

func (model *providerLiveModel) canEdit() bool {
	_, selected := model.selectedProvider()
	return selected && model.tab != freeProviderTab
}

func (model *providerLiveModel) canTest() bool {
	_, selected := model.selectedProvider()
	return selected
}

func (model *providerLiveModel) canDelete() bool {
	_, selected := model.selectedProvider()
	return selected && model.tab != freeProviderTab
}

func (model *providerLiveModel) providerActions() []string {
	switch model.tab {
	case oauthProviderTab:
		return []string{model.ui.t("controls.providerToggle"), model.ui.t("controls.providerDelete"), model.ui.t("controls.providerTest")}
	case freeProviderTab:
		return []string{model.ui.t("controls.providerTest")}
	default:
		return []string{model.ui.t("controls.providerEdit"), model.ui.t("controls.providerDelete"), model.ui.t("controls.providerTest")}
	}
}

func (model *providerLiveModel) runAction() tea.Cmd {
	if model.tab == customProviderTab && model.cursor < 2 {
		return model.action(model.add)
	}
	if model.tab == oauthProviderTab && model.cursor == 0 {
		return model.action(model.addOAuth)
	}
	if model.tab == freeProviderTab {
		return model.testAction()
	}
	return model.action(model.edit)
}

func (model *providerLiveModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
	model.lastAction = ""
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

func (model *providerLiveModel) testModel(input io.Reader, output io.Writer) (string, error) {
	item, ok := model.selectedProvider()
	if !ok {
		return "", nil
	}
	var available struct {
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := model.ui.request(http.MethodGet, "/api/providers/"+url.PathEscape(item.ID)+"/test-models", nil, &available); err != nil {
		return "", fmt.Errorf("%s: %w", item.Name, err)
	}
	if len(available.Models) == 0 {
		return "", errors.New(model.ui.t("form.noTestModels"))
	}
	options := make([]huh.Option[string], 0, len(available.Models))
	modelName := available.Models[0].ID
	for _, availableModel := range available.Models {
		label := availableModel.Name
		if label == "" {
			label = availableModel.ID
		}
		options = append(options, huh.NewOption(label, availableModel.ID))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title(model.ui.t("form.testModel")).Options(options...).Value(&modelName),
	))
	if err := model.ui.runHuhIO(form, input, output); err != nil {
		return "", err
	}
	var result struct {
		OK        bool   `json:"ok"`
		LatencyMs int64  `json:"latencyMs"`
		Error     string `json:"error"`
	}
	if err := model.ui.request(http.MethodPost, "/api/models/test", map[string]string{"model": strings.TrimSpace(modelName)}, &result); err != nil {
		return "", fmt.Errorf("%s · %s: %w", item.Name, modelName, err)
	}
	if !result.OK {
		message := result.Error
		if message == "" {
			message = model.ui.t("notice.modelTestFailed")
		}
		return "", fmt.Errorf("%s · %s: %s", item.Name, modelName, message)
	}
	return fmt.Sprintf("%s: %s · %s (%dms)", model.ui.t("notice.modelTestPassed"), item.Name, modelName, result.LatencyMs), nil
}

func (model *providerLiveModel) testAction() tea.Cmd {
	model.testing = true
	model.err = nil
	command := model.action(model.testModel)
	model.lastAction = "test"
	return command
}

func (model *providerLiveModel) selectedProvider() (provider, bool) {
	var items []provider
	start := 0
	switch model.tab {
	case customProviderTab:
		items, start = model.custom, 2
	case oauthProviderTab:
		items, start = model.oauthConnections, 1
	case freeProviderTab:
		items = model.free
	case apiKeyProviderTab:
		items = model.apiKeys
	}
	index := model.cursor - start
	if index < 0 || index >= len(items) {
		return provider{}, false
	}
	return items[index], true
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

func (model *providerLiveModel) refreshCmd() tea.Cmd {
	ui := model.ui
	return func() tea.Msg { return loadProviders(ui) }
}

func loadProviders(ui *UI) providerDataMsg {
	var response providersResponse
	if err := ui.request(http.MethodGet, "/api/providers", nil, &response); err != nil {
		return providerDataMsg{err: err}
	}
	var credentials []oauthProvider
	if err := ui.request(http.MethodGet, "/api/oauth", nil, &credentials); err != nil {
		return providerDataMsg{err: err}
	}
	custom, apiKeys := []provider{}, []provider{}
	oauthConnections := splitOAuthAccounts(response.Connections)
	for _, item := range response.Connections {
		if item.OAuthID != "" || item.ID == "codex" {
			continue
		}
		if item.APIType == "openai" || item.APIType == "anthropic" {
			custom = append(custom, item)
		} else {
			apiKeys = append(apiKeys, item)
		}
	}
	free := []provider{}
	for _, name := range freeProviderNames {
		for _, item := range response.Connections {
			if item.ID == name {
				free = append(free, item)
			}
		}
	}
	sort.Slice(custom, func(left, right int) bool { return custom[left].ID < custom[right].ID })
	sort.Slice(oauthConnections, func(left, right int) bool { return oauthConnections[left].ID < oauthConnections[right].ID })
	sort.Slice(free, func(left, right int) bool { return free[left].ID < free[right].ID })
	sort.Slice(apiKeys, func(left, right int) bool { return apiKeys[left].ID < apiKeys[right].ID })
	return providerDataMsg{custom: custom, oauth: credentials, oauthConnections: oauthConnections, free: free, apiKeys: apiKeys}
}

func splitOAuthAccounts(connections []provider) []provider {
	result := []provider{}
	for _, item := range connections {
		if item.ID == "codex" && len(item.Accounts) > 0 {
			for _, account := range item.Accounts {
				connection := item
				connection.ID, connection.Name, connection.OAuthID = account.ID, account.Name, account.OAuthID
				connection.Enabled, connection.Accounts = account.Enabled, []providerAccount{account}
				result = append(result, connection)
			}
			continue
		}
		if item.OAuthID != "" || item.ID == "codex" {
			result = append(result, item)
		}
	}
	return result
}

func (model *providerLiveModel) oauthCredential(item provider) *oauthProvider {
	for index := range model.oauth {
		if model.oauth[index].ID == item.OAuthID || model.oauth[index].Provider == item.ID {
			return &model.oauth[index]
		}
	}
	return nil
}

func providerRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return providerRefreshMsg{} })
}
