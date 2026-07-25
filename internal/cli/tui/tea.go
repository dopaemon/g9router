package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type menuItem struct{ title, description string }

func (item menuItem) FilterValue() string { return item.title }
func (item menuItem) Title() string       { return item.title }
func (item menuItem) Description() string { return item.description }

type teaModel struct {
	baseURL string
	list    list.Model
	width   int
	status  string
}

func newTeaModel(baseURL string) teaModel {
	items := []list.Item{
		menuItem{"Providers", "Manage upstream providers and authentication"},
		menuItem{"API Keys", "Create, inspect, and rotate gateway keys"},
		menuItem{"Combos", "Compose reusable model routes"},
		menuItem{"CLI Tools", "Configure supported coding assistants"},
		menuItem{"Settings", "Inspect runtime settings"},
		menuItem{"OAuth", "Connect browser-based provider accounts"},
	}
	menu := list.New(items, list.NewDefaultDelegate(), 0, 0)
	menu.Title = "9Router Control Center"
	menu.SetShowHelp(false)
	menu.SetShowStatusBar(false)
	menu.SetShowPagination(false)
	return teaModel{baseURL: baseURL, list: menu, status: "Ready"}
}

func (model teaModel) Init() tea.Cmd { return nil }

func (model teaModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = message.Width
		model.list.SetSize(message.Width-4, message.Height-9)
	case tea.KeyMsg:
		switch strings.ToLower(message.String()) {
		case "q", "ctrl+c":
			return model, tea.Quit
		case "enter":
			model.status = fmt.Sprintf("Selected %s — press q to exit", model.list.SelectedItem().(menuItem).title)
		case "b", "esc":
			model.status = "Ready"
		}
	}
	var command tea.Cmd
	model.list, command = model.list.Update(message)
	return model, command
}

func (model teaModel) View() string {
	width := model.width
	if width < 60 {
		width = 60
	}
	header := styles.Brand.Render(gradient("9ROUTER")) + "  " + styles.Subtitle.Render("AI gateway control center")
	intro := styles.Title.Render("Make every route feel intentional.") + "\n" + styles.Subtitle.Render(model.baseURL)
	body := styles.Panel.Width(width - 4).Render(model.list.View())
	status := styles.Footer.Width(width - 4).Render(styles.Muted.Render("↑/↓ navigate  enter select  q quit") + "  " + styles.Success.Render(model.status))
	return "\n" + header + "\n\n" + intro + "\n\n" + body + "\n" + status + "\n"
}

func runInteractive(baseURL string, out io.Writer) error {
	program := tea.NewProgram(newTeaModel(baseURL), tea.WithOutput(out))
	_, err := program.Run()
	return err
}
