package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type screen struct {
	title       string
	description string
	path        string
}

var screens = []screen{
	{title: "Providers", description: "Upstream connections and authentication", path: "/api/providers"},
	{title: "API Keys", description: "Gateway credentials and access control", path: "/api/keys"},
	{title: "Combos", description: "Reusable model routing combinations", path: "/api/combos"},
	{title: "CLI Tools", description: "Coding assistant status and setup", path: "/api/cli-tools/all-statuses"},
	{title: "Settings", description: "Runtime configuration and system state", path: "/api/settings"},
	{title: "OAuth", description: "Connected provider credentials", path: "/api/oauth"},
}

type screenModel struct {
	baseURL  string
	client   *http.Client
	output   io.Writer
	menu     teaModel
	current  *screen
	viewport viewport.Model
	spinner  spinner.Model
	form     *formModel
	items    []resourceItem
	selected int
	notice   string
	raw      string
	loading  bool
	err      error
	width    int
	height   int
}

type screenLoadedMsg struct {
	content string
	err     error
}

type screenTickMsg struct{}

type resourceItem struct {
	id, label, detail, status string
}

func newScreenModel(baseURL string, output io.Writer, client *http.Client) screenModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	model := screenModel{baseURL: baseURL, client: client, output: output, menu: newTeaModel(baseURL), spinner: spin}
	model.viewport = viewport.New(0, 0)
	return model
}

func (model screenModel) Init() tea.Cmd { return nil }

func (model screenModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if model.current == nil {
		updated, command := model.menu.Update(message)
		model.menu = updated.(teaModel)
		if model.menu.selected != "" {
			for index := range screens {
				if screens[index].title == model.menu.selected {
					model.current = &screens[index]
					model.loading = true
					return model, tea.Batch(loadScreen(model.baseURL, model.client, model.current.path), model.spinner.Tick)
				}
			}
		}
		return model, command
	}
	if model.form != nil {
		if key, ok := message.(tea.KeyMsg); ok && strings.ToLower(key.String()) == "esc" {
			model.form = nil
			return model, nil
		}
		updated, command := model.form.update(message)
		model.form = &updated
		if saved, ok := message.(formSavedMsg); ok {
			model.form.busy = false
			if saved.err != nil {
				model.form.err = saved.err
				return model, nil
			}
			model.form = nil
			model.loading = true
			return model, loadScreen(model.baseURL, model.client, model.current.path)
		}
		return model, command
	}

	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width, model.height = message.Width, message.Height
		model.viewport.Width = message.Width - 8
		model.viewport.Height = message.Height - 11
	case screenLoadedMsg:
		model.loading, model.err = false, message.err
		model.raw = message.content
		model.items = resourceItems(model.current.title, message.content)
		model.selected = 0
		model.viewport.SetContent(model.content(message.content))
	case spinner.TickMsg:
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(message)
		if model.loading {
			return model, tea.Batch(command, model.spinner.Tick)
		}
	case tea.KeyMsg:
		switch strings.ToLower(message.String()) {
		case "q", "ctrl+c":
			return model, tea.Quit
		case "b", "esc":
			model.current = nil
			model.menu.selected = ""
			model.err = nil
		case "r":
			model.loading = true
			return model, tea.Batch(loadScreen(model.baseURL, model.client, model.current.path), model.spinner.Tick)
		case "up", "k":
			if len(model.items) > 0 && model.selected > 0 {
				model.selected--
				model.viewport.SetContent(model.content(model.raw))
			}
		case "down", "j":
			if len(model.items) > 0 && model.selected < len(model.items)-1 {
				model.selected++
				model.viewport.SetContent(model.content(model.raw))
			}
		case "enter":
			if len(model.items) > 0 {
				model.notice = "Selected " + model.items[model.selected].label
			}
		case "a":
			switch model.current.title {
			case "Providers":
				form := newForm("Add provider", model.baseURL, "/api/providers", model.client, "Name", "Base URL", "API key")
				model.form = &form
			case "API Keys":
				form := newForm("Create API key", model.baseURL, "/api/keys", model.client, "Name")
				model.form = &form
			default:
				model.err = fmt.Errorf("add action is not available for %s", model.current.title)
			}
			return model, nil
		default:
			var command tea.Cmd
			model.viewport, command = model.viewport.Update(message)
			return model, command
		}
	}
	return model, nil
}

func (model screenModel) View() string {
	if model.current == nil {
		return model.menu.View()
	}
	width := model.width
	if width < 60 {
		width = 60
	}
	header := styles.Brand.Render(gradient("9ROUTER")) + "  " + styles.Subtitle.Render(model.baseURL)
	title := styles.Title.Render(model.current.title) + "\n" + styles.Subtitle.Render(model.current.description)
	content := styles.Panel.Width(width - 4).Render(model.renderContent())
	footerText := "↑/↓ select  enter open  r refresh  b back  q quit"
	if model.notice != "" {
		footerText += "  " + styles.Success.Render(model.notice)
	}
	footer := styles.Footer.Width(width - 4).Render(styles.Muted.Render(footerText))
	return "\n" + header + "\n\n" + title + "\n\n" + content + "\n" + footer + "\n"
}

func (model screenModel) renderContent() string {
	if model.form != nil {
		return model.form.view()
	}
	if model.loading {
		return model.spinner.View() + " " + styles.Subtitle.Render("Loading "+model.current.title+"…")
	}
	if model.err != nil {
		return styles.Error.Render("✗ "+model.err.Error()) + "\n\n" + styles.Muted.Render("Press r to retry")
	}
	return model.viewport.View()
}

func (model screenModel) content(raw string) string {
	if len(model.items) == 0 {
		return formatScreenContent(model.current.title, raw)
	}
	lines := make([]string, 0, len(model.items)+2)
	for index, item := range model.items {
		marker := "  "
		if index == model.selected {
			marker = styles.Selected.Render("▸ ")
		}
		status := item.status
		if status == "enabled" || status == "active" || status == "installed" {
			status = styles.Success.Render(status)
		} else if status != "" {
			status = styles.Warning.Render(status)
		}
		lines = append(lines, marker+styles.Item.Render(fmt.Sprintf("%-24s %-42s %s", item.label, item.detail, status)))
	}
	hint := "enter select"
	if model.current.title == "Providers" || model.current.title == "API Keys" {
		hint += "  a add"
	}
	return strings.Join(lines, "\n") + "\n\n" + styles.Muted.Render(hint)
}

func resourceItems(title, content string) []resourceItem {
	var value any
	if json.Unmarshal([]byte(content), &value) != nil {
		return nil
	}
	result := make([]resourceItem, 0)
	if title == "Providers" || title == "API Keys" || title == "Combos" {
		key := map[string]string{"Providers": "connections", "API Keys": "keys", "Combos": "combos"}[title]
		items, _ := value.(map[string]any)[key].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			label, _ := item["name"].(string)
			if label == "" {
				label, _ = item["id"].(string)
			}
			detail, _ := item["baseURL"].(string)
			status := ""
			if enabled, ok := item["enabled"].(bool); ok {
				status = map[bool]string{true: "enabled", false: "disabled"}[enabled]
			}
			if active, ok := item["isActive"].(bool); ok {
				status = map[bool]string{true: "active", false: "inactive"}[active]
			}
			if models, ok := item["models"].([]any); ok {
				detail = fmt.Sprintf("%d models", len(models))
			}
			id, _ := item["id"].(string)
			result = append(result, resourceItem{id: id, label: label, detail: detail, status: status})
		}
	}
	if title == "CLI Tools" {
		items, _ := value.(map[string]any)
		for label, raw := range items {
			item, _ := raw.(map[string]any)
			installed, _ := item["installed"].(bool)
			configured, _ := item["configured"].(bool)
			status := "not installed"
			if installed && configured {
				status = "configured"
			} else if installed {
				status = "installed"
			}
			result = append(result, resourceItem{label: label, status: status})
		}
		sort.Slice(result, func(left, right int) bool { return result[left].label < result[right].label })
	}
	return result
}

func loadScreen(baseURL string, client *http.Client, path string) tea.Cmd {
	return func() tea.Msg {
		request, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+path, nil)
		if err != nil {
			return screenLoadedMsg{err: err}
		}
		response, err := client.Do(request)
		if err != nil {
			return screenLoadedMsg{err: err}
		}
		defer response.Body.Close()
		var value any
		if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
			return screenLoadedMsg{err: err}
		}
		if response.StatusCode >= 400 {
			return screenLoadedMsg{err: fmt.Errorf("HTTP %s", response.Status)}
		}
		data, err := json.MarshalIndent(value, "", "  ")
		return screenLoadedMsg{content: string(data), err: err}
	}
}

func formatScreenContent(title, content string) string {
	var value any
	if json.Unmarshal([]byte(content), &value) != nil {
		return content
	}
	switch title {
	case "Providers":
		return formatProviders(value)
	case "API Keys":
		return formatKeys(value)
	case "Combos":
		return formatCombos(value)
	case "CLI Tools":
		return formatCLITools(value)
	case "Settings":
		return formatSettings(value)
	case "OAuth":
		return formatOAuth(value)
	default:
		data, _ := json.MarshalIndent(value, "", "  ")
		return string(data)
	}
}

func formatProviders(value any) string {
	connections, _ := value.(map[string]any)["connections"].([]any)
	if len(connections) == 0 {
		return styles.Muted.Render("No providers configured.") + "\n\n" + styles.Success.Render("Press a to add one")
	}
	lines := []string{styles.Title.Render("NAME") + "        " + styles.Title.Render("BASE URL") + "                         " + styles.Title.Render("STATUS")}
	for _, raw := range connections {
		item, _ := raw.(map[string]any)
		name, _ := item["name"].(string)
		if name == "" {
			name, _ = item["id"].(string)
		}
		baseURL, _ := item["baseURL"].(string)
		enabled, _ := item["enabled"].(bool)
		status := styles.Error.Render("disabled")
		if enabled {
			status = styles.Success.Render("enabled")
		}
		lines = append(lines, fmt.Sprintf("%-12s %-42s %s", name, baseURL, status))
	}
	return strings.Join(lines, "\n") + "\n\n" + styles.Muted.Render("a add  e edit  d delete  t test")
}

func formatKeys(value any) string {
	keys, _ := value.(map[string]any)["keys"].([]any)
	if len(keys) == 0 {
		return styles.Muted.Render("No API keys configured.") + "\n\n" + styles.Success.Render("Press a to create one")
	}
	lines := []string{styles.Title.Render("NAME") + "                         " + styles.Title.Render("STATUS")}
	for _, raw := range keys {
		item, _ := raw.(map[string]any)
		name, _ := item["name"].(string)
		active, _ := item["isActive"].(bool)
		status := styles.Error.Render("inactive")
		if active {
			status = styles.Success.Render("active")
		}
		lines = append(lines, fmt.Sprintf("%-32s %s", name, status))
	}
	return strings.Join(lines, "\n") + "\n\n" + styles.Muted.Render("a create  d delete")
}

func formatCombos(value any) string {
	combos, _ := value.(map[string]any)["combos"].([]any)
	if len(combos) == 0 {
		return styles.Muted.Render("No combos configured.")
	}
	lines := []string{styles.Title.Render("NAME") + "                         " + styles.Title.Render("MODELS")}
	for _, raw := range combos {
		item, _ := raw.(map[string]any)
		name, _ := item["name"].(string)
		models, _ := item["models"].([]any)
		lines = append(lines, fmt.Sprintf("%-32s %d", name, len(models)))
	}
	return strings.Join(lines, "\n") + "\n\n" + styles.Muted.Render("a add  e edit  d delete")
}

func formatCLITools(value any) string {
	tools, _ := value.(map[string]any)
	if len(tools) == 0 {
		return styles.Muted.Render("No CLI tool status available.")
	}
	lines := []string{styles.Title.Render("TOOL") + "                         " + styles.Title.Render("INSTALLED") + "   " + styles.Title.Render("CONFIGURED")}
	for name, raw := range tools {
		item, _ := raw.(map[string]any)
		installed, _ := item["installed"].(bool)
		configured, _ := item["configured"].(bool)
		lines = append(lines, fmt.Sprintf("%-32s %-11t %t", name, installed, configured))
	}
	sort.Strings(lines[1:])
	return strings.Join(lines, "\n") + "\n\n" + styles.Muted.Render("r refresh  use legacy setup actions from CLI Tools")
}

func formatSettings(value any) string {
	settings, _ := value.(map[string]any)
	if len(settings) == 0 {
		return styles.Muted.Render("No settings available.")
	}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{styles.Title.Render("SETTING") + "                         " + styles.Title.Render("VALUE")}
	for _, key := range keys {
		value, _ := json.Marshal(settings[key])
		lines = append(lines, fmt.Sprintf("%-32s %s", key, strings.Trim(string(value), `"`)))
	}
	return strings.Join(lines, "\n")
}

func formatOAuth(value any) string {
	credentials, _ := value.(map[string]any)
	if len(credentials) == 0 {
		return styles.Muted.Render("No OAuth credentials connected.") + "\n\n" + styles.Success.Render("Use the legacy OAuth flow to connect one")
	}
	lines := []string{styles.Title.Render("ID") + "                         " + styles.Title.Render("PROVIDER") + "              " + styles.Title.Render("STATUS")}
	for id, raw := range credentials {
		item, _ := raw.(map[string]any)
		provider, _ := item["provider"].(string)
		if provider == "" {
			provider, _ = item["type"].(string)
		}
		status := styles.Success.Render("connected")
		if active, ok := item["active"].(bool); ok && !active {
			status = styles.Warning.Render("inactive")
		}
		lines = append(lines, fmt.Sprintf("%-28s %-24s %s", id, provider, status))
	}
	sort.Strings(lines[1:])
	return strings.Join(lines, "\n")
}

func runFullInteractive(baseURL string, output io.Writer, client *http.Client) error {
	program := tea.NewProgram(newScreenModel(baseURL, output, client), tea.WithOutput(output))
	_, err := program.Run()
	return err
}
