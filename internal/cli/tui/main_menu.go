package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type mainMenuModel struct {
	ui        *UI
	items     []string
	cursor    int
	selected  string
	menuTop   int
	menuLeft  int
	menuWidth int
}

const g9routerBanner = `██████╗  █████╗ ██████╗  ██████╗ ██╗   ██╗████████╗███████╗██████╗
██╔════╝ ██╔══██╗██╔══██╗██╔═══██╗██║   ██║╚══██╔══╝██╔════╝██╔══██╗
██║  ███╗╚██████║██████╔╝██║   ██║██║   ██║   ██║   █████╗  ██████╔╝
██║   ██║ ╚═══██║██╔══██╗██║   ██║██║   ██║   ██║   ██╔══╝  ██╔══██╗
╚██████╔╝ █████╔╝██║  ██║╚██████╔╝╚██████╔╝   ██║   ███████╗██║  ██║
 ╚═════╝  ╚════╝ ╚═╝  ╚═╝ ╚═════╝  ╚═════╝    ╚═╝   ╚══════╝╚═╝  ╚═╝`

func (ui *UI) mainMenuChoice(items []string) (string, error) {
	if !isInteractiveWriter(ui.Out) {
		return "", nil
	}
	model := &mainMenuModel{ui: ui, items: items}
	err := ui.runTea(model)
	return model.selected, err
}

func (ui *UI) runTea(model tea.Model) error {
	options := []tea.ProgramOption{tea.WithInput(ui.In), tea.WithOutput(ui.Out), tea.WithAltScreen()}
	if !accessibleMode(ui.In) {
		options = append(options, tea.WithMouseCellMotion())
	}
	_, err := tea.NewProgram(sizedModel{ui: ui, model: model}, options...).Run()
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
			model.cursor = moveIndex(model.cursor, len(model.items), -1)
		case "down", "j":
			model.cursor = moveIndex(model.cursor, len(model.items), 1)
		case "left", "h":
			model.cursor = moveIndex(model.cursor, len(model.items), -1)
		case "right", "l":
			model.cursor = moveIndex(model.cursor, len(model.items), 1)
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
	if model.ui.viewClipped || x < model.menuLeft || x >= model.menuLeft+model.menuWidth || y < model.menuTop {
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
	menu := strings.Join(rows, "\n")
	model.menuTop = 2 + 1 + 2 + 2
	model.menuLeft = 1 + 2 + 1 + 2
	model.menuWidth = menuWidth
	menuCard := model.ui.innerStyle().Render(menu)
	banner := truncateBanner(g9routerBanner, max(1, menuWidth+4))
	controls := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumns(
		"↑↓/jk "+model.ui.t("controls.menuMove"),
		"Enter "+model.ui.t("controls.menuSelect"),
		fmt.Sprintf("1–%d "+model.ui.t("controls.menuDirect"), len(model.items)),
		"q "+model.ui.t("controls.menuExit"),
	))
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.title")) + "\n\n" + banner + "\n\n" + menuCard + "\n\n" + controls)
}

func truncateBanner(value string, width int) string {
	width = max(1, width)
	lines := strings.Split(value, "\n")
	maxWidth := 0
	for _, line := range lines {
		maxWidth = max(maxWidth, lipgloss.Width(line))
	}
	if maxWidth > width {
		lines = []string{"G9Router"}
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#F472B6"))
	center := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)
	for index := range lines {
		line := ansi.Truncate(lines[index], width, "")
		lines[index] = center.Render(style.Render(line))
	}
	return strings.Join(lines, "\n")
}

func (model *mainMenuModel) menuItem(index int) string {
	label := ansi.Truncate(fmt.Sprintf("%d  %s", index+1, model.items[index]), max(1, model.ui.innerWidth()-2), "…")
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
