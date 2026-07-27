package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

type mainMenuModel struct {
	ui       *UI
	items    []string
	cursor   int
	selected string
	bannerX  float64
	bannerV  float64
	spring   harmonica.Spring
}

type mainMenuTickMsg struct{}

const (
	bannerArea = 72
)

func (ui *UI) mainMenuChoice(items []string) (string, error) {
	if !isInteractiveWriter(ui.Out) {
		return "", nil
	}
	model := &mainMenuModel{ui: ui, items: items, spring: harmonica.NewSpring(harmonica.FPS(60), 3, 1)}
	err := ui.runTea(model)
	return model.selected, err
}

func (ui *UI) runTea(model tea.Model) error {
	_, err := tea.NewProgram(model, tea.WithInput(ui.In), tea.WithOutput(ui.Out), tea.WithAltScreen(), tea.WithMouseAllMotion()).Run()
	return err
}

func (model *mainMenuModel) Init() tea.Cmd { return mainMenuTick() }

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
	if _, ok := message.(mainMenuTickMsg); ok {
		model.bannerX, model.bannerV = model.spring.Update(model.bannerX, model.bannerV, 3)
		if model.bannerX < 0 {
			model.bannerX, model.bannerV = 0, 0
		}
		if model.bannerX > 3 {
			model.bannerX, model.bannerV = 3, 0
		}
		if math.Abs(model.bannerX-3) < 0.05 && math.Abs(model.bannerV) < 0.05 {
			model.bannerX, model.bannerV = 3, 0
			return model, nil
		}
		return model, mainMenuTick()
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
	if y < 15 {
		return -1
	}
	index := y - 15
	if index < 0 || index >= len(model.items) {
		return -1
	}
	return index
}

func (model *mainMenuModel) View() string {
	rows := make([]string, 0, len(model.items))
	column := lipgloss.NewStyle().Width(60)
	rowCount := len(model.items)
	for row := 0; row < rowCount; row++ {
		rows = append(rows, column.Render(model.menuItem(row)))
	}
	banner := lipgloss.NewStyle().Width(bannerArea).Align(lipgloss.Left).Render(slidingBanner(int(math.Round(model.bannerX))))
	menu := strings.Join(rows, "\n") + "\n\n" + mutedStyle.Render("↑↓/jk move  Enter select  1–8 direct  q exit")
	menuCard := innerCardStyle.Render(menu)
	return outerCardStyle.Render(banner + "\n\n" + cardTitleStyle.Render(model.ui.t("menu.title")) + "\n\n" + menuCard)
}

func slidingBanner(offset int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > 3 {
		offset = 3
	}
	rows := strings.Split(cliBanner, "\n")
	for index, row := range rows {
		line := []rune(strings.Repeat(" ", offset) + row)
		if len(line) > bannerArea {
			line = line[:bannerArea]
		}
		rows[index] = gradientText(string(line))
	}
	return strings.Join(rows, "\n")
}

func mainMenuTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(time.Time) tea.Msg { return mainMenuTickMsg{} })
}

func (model *mainMenuModel) menuItem(index int) string {
	label := fmt.Sprintf("%d  %s", index+1, model.items[index])
	if index == model.cursor {
		label = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1).Render(label)
	} else {
		label = cardTitleStyle.Render(label)
	}
	return lipgloss.NewStyle().Width(60).Padding(0, 1).Render(label)
}
