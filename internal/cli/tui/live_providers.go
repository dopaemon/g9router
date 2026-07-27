package tui

import (
	"bufio"
	"errors"
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
}

var freeProviderNames = []string{"mimo-free"}

func (ui *UI) liveProviders() error {
	EnableColors(ui.Out)
	model := providerLiveModel{ui: ui}
	model.refresh()
	return ui.runTea(&model)
}

func (model *providerLiveModel) Init() tea.Cmd { return providerRefresh() }

func (model *providerLiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= model.itemCount() {
			model.cursor = index - 1
			return model, model.runAction()
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "tab", "right", "l":
			model.tab = (model.tab + 1) % 4
			model.cursor = 0
		case "shift+tab", "left", "h":
			model.tab = (model.tab + 3) % 4
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

func (model *providerLiveModel) View() string {
	if model.err != nil {
		return outerCardStyle.Render(cardTitleStyle.Render(model.ui.t("menu.providers")) + "\n\n" + model.err.Error() + "\n\n" + mutedStyle.Render("Press q or Esc to go back."))
	}
	tabs := []string{"Custom", "OAuth", "Free Tier", "API Key"}
	tabLine := make([]string, len(tabs))
	for index, tab := range tabs {
		style := mutedStyle.Padding(0, 1)
		if providerTab(index) == model.tab {
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1)
		}
		tabLine[index] = style.Render(tab)
	}
	content := model.cardContent()
	column := lipgloss.NewStyle().Width(30)
	controlRows := []string{
		lipgloss.JoinHorizontal(lipgloss.Top, column.Render(mutedStyle.Render("↑↓/jk move")), column.Render(mutedStyle.Render("Tab switch card"))),
		lipgloss.JoinHorizontal(lipgloss.Top, column.Render(mutedStyle.Render("Enter select")), column.Render(mutedStyle.Render("e edit"))),
		lipgloss.JoinHorizontal(lipgloss.Top, column.Render(mutedStyle.Render("d delete")), column.Render(mutedStyle.Render("q back"))),
	}
	controls := strings.Join(controlRows, "\n")
	if model.notice != "" {
		controls += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80")).Render(model.notice)
	}
	controlCard := innerCardStyle.Render(cardTitleStyle.Render("Controls") + "\n" + controls)
	banner := lipgloss.NewStyle().Width(78).Align(lipgloss.Center).Render(gradientText(cliBanner))
	return outerCardStyle.Render(banner + "\n\n" + cardTitleStyle.Render(model.ui.t("menu.providers")) + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, tabLine...) + "\n\n" + content + "\n\n" + controlCard)
}

func (model *providerLiveModel) cardContent() string {
	var title string
	var rows []string
	switch model.tab {
	case customProviderTab:
		title = "Custom Providers (OpenAI/Anthropic Compatible)"
		rows = []string{"Add Anthropic Compatible", "Add OpenAI Compatible"}
		for _, item := range model.custom {
			rows = append(rows, item.Name+" ("+item.ID+")")
		}
	case oauthProviderTab:
		title = "OAuth Providers"
		rows = []string{"Add OAuth provider"}
		for _, item := range model.oauthConnections {
			name := item.Name
			if name == "" {
				name = item.ID
			}
			rows = append(rows, name+" ("+item.ID+") ["+statusText(item.Enabled)+"]")
		}
	case freeProviderTab:
		title = "Free Tier Providers"
		for _, name := range freeProviderNames {
			item := model.find(model.free, name)
			rows = append(rows, name+" ["+statusText(item != nil && item.Enabled)+"]")
		}
	case apiKeyProviderTab:
		title = "API Key Providers"
		for _, item := range model.apiKeys {
			rows = append(rows, item.Name+" ("+item.ID+") ["+statusText(item.Enabled)+"]")
		}
	}
	if len(rows) == 0 {
		rows = []string{"No providers found."}
	}
	items := make([]string, len(rows))
	for index, row := range rows {
		items[index] = providerMenuItem(index, row, index == model.cursor)
	}
	return innerCardStyle.Render(cardTitleStyle.Render(title) + "\n" + strings.Join(items, "\n"))
}

func providerMenuItem(index int, label string, selected bool) string {
	text := string(rune('1'+index)) + "  " + label
	if selected {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1).Render(text)
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
	if err := model.ui.promptProviderIO(item, false, input, output); err != nil {
		return "", err
	}
	return "Provider added", nil
}

func (model *providerLiveModel) addOAuth(input io.Reader, output io.Writer) (string, error) {
	if model.tab != oauthProviderTab || model.cursor != 0 {
		return "", nil
	}
	oldIn, oldOut := model.ui.In, model.ui.Out
	model.ui.In, model.ui.Out = input, output
	err := model.ui.loginOAuthProvider(bufio.NewReader(input))
	model.ui.In, model.ui.Out = oldIn, oldOut
	return "OAuth provider added", err
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
		return "Provider updated", model.ui.request(http.MethodPut, "/api/providers/"+url.PathEscape(item.ID), map[string]bool{"enabled": !item.Enabled}, nil)
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
	return "Provider updated", model.ui.promptProviderIO(&current.Connection, true, input, output)
}

func (model *providerLiveModel) delete(input io.Reader, output io.Writer) (string, error) {
	if model.tab == customProviderTab {
		index := model.cursor - 2
		if index < 0 || index >= len(model.custom) {
			return "", nil
		}
		item := model.custom[index]
		return "Provider deleted", model.deleteProvider(item.ID, input, output)
	}
	if model.tab == apiKeyProviderTab && model.cursor < len(model.apiKeys) {
		return "Provider deleted", model.deleteProvider(model.apiKeys[model.cursor].ID, input, output)
	}
	if model.tab == oauthProviderTab {
		index := model.cursor - 1
		if index < 0 || index >= len(model.oauthConnections) {
			return "", nil
		}
		return "OAuth provider deleted", model.deleteOAuthProvider(model.oauthConnections[index], input, output)
	}
	return "", nil
}

func (model *providerLiveModel) deleteProvider(id string, input io.Reader, output io.Writer) error {
	ok, err := confirmHuh(input, output, "Delete provider "+id+"?", func(form *huh.Form) error {
		return model.ui.runHuhIO(form, input, output)
	})
	if err != nil || !ok {
		return err
	}
	return model.ui.request(http.MethodDelete, "/api/providers?id="+url.QueryEscape(id), nil, nil)
}

func (model *providerLiveModel) deleteOAuthProvider(item provider, input io.Reader, output io.Writer) error {
	ok, err := confirmHuh(input, output, "Delete OAuth provider?", func(form *huh.Form) error {
		return model.ui.runHuhIO(form, input, output)
	})
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
