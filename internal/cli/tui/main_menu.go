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
	_, err := tea.NewProgram(model, tea.WithInput(ui.In), tea.WithOutput(ui.Out), tea.WithAltScreen()).Run()
	return err
}

func (model *mainMenuModel) Init() tea.Cmd { return mainMenuTick() }

func (model *mainMenuModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := message.(mainMenuTickMsg); ok {
		model.bannerX, model.bannerV = model.spring.Update(model.bannerX, model.bannerV, 4)
		if model.bannerX < 0 {
			model.bannerX, model.bannerV = 0, 0
		}
		if model.bannerX > 4 {
			model.bannerX, model.bannerV = 4, 0
		}
		if math.Abs(model.bannerX-4) < 0.05 && math.Abs(model.bannerV) < 0.05 {
			model.bannerX, model.bannerV = 4, 0
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

func (model *mainMenuModel) View() string {
	rows := make([]string, 0, 3)
	column := lipgloss.NewStyle().Width(38)
	rowCount := (len(model.items) + 1) / 2
	startRow := model.cursor / 2
	if startRow > 2 {
		startRow -= 2
	}
	if startRow+3 > rowCount {
		startRow = rowCount - 3
	}
	if startRow < 0 {
		startRow = 0
	}
	for row := startRow; row < rowCount && row < startRow+3; row++ {
		index := row * 2
		left := model.menuItem(index)
		right := ""
		if index+1 < len(model.items) {
			right = model.menuItem(index + 1)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, column.Render(left), column.Render(right)))
	}
	banner := lipgloss.NewStyle().Width(bannerArea).Align(lipgloss.Left).Render(slidingBanner(int(math.Round(model.bannerX))))
	controls := innerCardStyle.Render(cardTitleStyle.Render("Controls") + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("↑↓/←→ move by number")),
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("Enter select  1–9 direct  q exit")),
	))
	return outerCardStyle.Render(banner + "\n\n" + cardTitleStyle.Render(model.ui.t("menu.title")) + "\n\n" + lipgloss.JoinVertical(lipgloss.Left, rows...) + "\n\n" + controls)
}

func slidingBanner(offset int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > 10 {
		offset = 10
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
	return innerCardStyle.Width(31).Padding(0, 1).Render(label)
}
