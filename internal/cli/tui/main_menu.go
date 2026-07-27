package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mainMenuModel struct {
	ui       *UI
	items    []string
	cursor   int
	selected string
	menuTop  int
}

const (
	bannerArea = 72
)

func (ui *UI) mainMenuChoice(items []string) (string, error) {
	if !isInteractiveWriter(ui.Out) {
		return "", nil
	}
	model := &mainMenuModel{ui: ui, items: items}
	err := ui.runTea(model)
	return model.selected, err
}

func (ui *UI) runTea(model tea.Model) error {
	_, err := tea.NewProgram(sizedModel{ui: ui, model: model}, tea.WithInput(ui.In), tea.WithOutput(ui.Out), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func (model *mainMenuModel) Init() tea.Cmd { return nil }

func (model *mainMenuModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if mouse, ok := message.(tea.MouseMsg); ok {
		if index := model.mouseItem(mouse.X, mouse.Y); index >= 0 {
			model.cursor = index
			if (mouse.Action == tea.MouseActionPress || mouse.Action == tea.MouseActionRelease) && mouse.Button == tea.MouseButtonLeft {
				model.selected = model.items[index]
				return model, tea.Quit
			}
		}
	}
	if key, ok := message.(tea.KeyMsg); ok {
		if index, err := strconv.Atoi(key.String()); err == nil && index >= 1 && index <= len(model.items) {
			model.cursor = index - 1
			model.selected = model.items[model.cursor]
			return model, tea.Quit
		}
		switch key.String() {
		case "up", "k":
			if model.cursor > 0 {
				model.cursor--
			}
		case "down", "j":
			if model.cursor+1 < len(model.items) {
				model.cursor++
			}
		case "left", "h":
			if model.cursor > 0 {
				model.cursor--
			}
		case "right", "l":
			if model.cursor+1 < len(model.items) {
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

func (model *mainMenuModel) mouseItem(x, y int) int {
	if y < model.menuTop {
		return -1
	}
	index := y - model.menuTop
	if index < 0 || index >= len(model.items) {
		return -1
	}
	return index
}

func (model *mainMenuModel) View() string {
	menuWidth := model.ui.innerWidth()
	rows := make([]string, 0, len(model.items))
	column := lipgloss.NewStyle().Width(menuWidth)
	rowCount := len(model.items)
	for row := 0; row < rowCount; row++ {
		rows = append(rows, column.Render(model.menuItem(row)))
	}
	bannerWidth := min(menuWidth, bannerArea)
	banner := lipgloss.NewStyle().Width(bannerWidth).Align(lipgloss.Left).Render(slidingBanner(0, bannerWidth))
	menu := strings.Join(rows, "\n") + "\n\n" + mutedStyle.Render(model.ui.t("common.menuControls"))
	model.menuTop = 2 + lipgloss.Height(banner) + 2 + 1 + 2 + 2
	menuCard := model.ui.innerStyle().Render(menu)
	return model.ui.outerStyle().Render(banner + "\n\n" + cardTitleStyle.Render(model.ui.t("menu.title")) + "\n\n" + menuCard)
}

func slidingBanner(offset, width int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > 3 {
		offset = 3
	}
	rows := strings.Split(cliBanner, "\n")
	for index, row := range rows {
		line := []rune(strings.Repeat(" ", offset) + row)
		if len(line) > width {
			line = line[:width]
		}
		rows[index] = gradientText(string(line))
	}
	return strings.Join(rows, "\n")
}

func (model *mainMenuModel) menuItem(index int) string {
	label := fmt.Sprintf("%d  %s", index+1, model.items[index])
	if index == model.cursor {
		label = focusStyle.Render(label)
	} else {
		label = cardTitleStyle.Render(label)
	}
	return lipgloss.NewStyle().Width(model.ui.innerWidth()).Padding(0, 1).Render(label)
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
