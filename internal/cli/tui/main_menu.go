package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mainMenuModel struct {
	ui       *UI
	items    []string
	cursor   int
	selected string
}

func (ui *UI) mainMenuChoice(items []string) (string, error) {
	if !isInteractiveWriter(ui.Out) {
		return "", nil
	}
	model := &mainMenuModel{ui: ui, items: items}
	_, err := tea.NewProgram(model, tea.WithInput(ui.In), tea.WithOutput(ui.Out)).Run()
	return model.selected, err
}

func (model *mainMenuModel) Init() tea.Cmd { return nil }

func (model *mainMenuModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		if index, err := strconv.Atoi(key.String()); err == nil && index >= 1 && index <= len(model.items) {
			model.cursor = index - 1
			model.selected = model.items[model.cursor]
			return model, tea.Quit
		}
		switch key.String() {
		case "up", "k":
			if model.cursor >= 2 {
				model.cursor -= 2
			}
		case "down", "j":
			if model.cursor+2 < len(model.items) {
				model.cursor += 2
			}
		case "left", "h":
			if model.cursor%2 == 1 {
				model.cursor--
			}
		case "right", "l":
			if model.cursor%2 == 0 && model.cursor+1 < len(model.items) {
				model.cursor++
			}
		case "enter", " ":
			model.selected = model.items[model.cursor]
			return model, tea.Quit
		case "q", "esc", "ctrl+c":
			model.selected = model.ui.t("menu.exit")
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model *mainMenuModel) View() string {
	rows := make([]string, 0, (len(model.items)+1)/2)
	column := lipgloss.NewStyle().Width(31)
	for index := 0; index < len(model.items); index += 2 {
		left := model.menuItem(index)
		right := ""
		if index+1 < len(model.items) {
			right = model.menuItem(index + 1)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, column.Render(left), column.Render(right)))
	}
	banner := lipgloss.NewStyle().Width(78).Align(lipgloss.Center).Render(gradientText(cliBanner))
	menu := innerCardStyle.Render(cardTitleStyle.Render(model.ui.t("menu.title")) + "\n" + lipgloss.JoinVertical(lipgloss.Left, rows...))
	controls := mutedStyle.Render("↑↓/jk move  ←→/hl switch  Enter select  q exit")
	return outerCardStyle.Render(banner + "\n\n" + menu + "\n\n" + controls)
}

func (model *mainMenuModel) menuItem(index int) string {
	label := fmt.Sprintf("%d  %s", index+1, model.items[index])
	if index == model.cursor {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1).Render(label)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Padding(0, 1).Render(label)
}
