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
		model.viewport.SetContent(formatScreenContent(model.current.title, message.content))
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
	footer := styles.Footer.Width(width - 4).Render(styles.Muted.Render("↑/↓ scroll  r refresh  b back  q quit"))
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

func runFullInteractive(baseURL string, output io.Writer, client *http.Client) error {
	program := tea.NewProgram(newScreenModel(baseURL, output, client), tea.WithOutput(output))
	_, err := program.Run()
	return err
}
