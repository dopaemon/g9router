package tui

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sizedModel struct {
	ui    *UI
	model tea.Model
}

func (model sizedModel) Init() tea.Cmd { return model.model.Init() }

func (model sizedModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		model.ui.width, model.ui.height = size.Width, size.Height
	}
	updated, command := model.model.Update(message)
	model.model = updated
	return model, command
}

func (model sizedModel) View() string { return model.model.View() }

func (ui *UI) cardWidth() int {
	width := ui.width - 4
	if width <= 0 {
		return 76
	}
	return max(20, min(width, 78))
}

func (ui *UI) innerWidth() int {
	width := ui.cardWidth() - 8
	if width < 12 {
		return 12
	}
	return width
}

func (ui *UI) columnWidth(columns int) int {
	if columns < 1 {
		return ui.innerWidth()
	}
	width := (ui.innerWidth() - columns + 1) / columns
	return max(1, width)
}

func (ui *UI) compact() bool { return ui.innerWidth() < 56 }

func (ui *UI) outerStyle() lipgloss.Style {
	return outerCardStyle.Width(ui.cardWidth())
}

func (ui *UI) innerStyle(width ...int) lipgloss.Style {
	cardWidth := ui.innerWidth()
	if len(width) > 0 {
		cardWidth = min(width[0], cardWidth)
	}
	return innerCardStyle.Width(max(1, cardWidth))
}

func (ui *UI) controlStyle() lipgloss.Style {
	return lipgloss.NewStyle().Width(ui.columnWidth(2))
}

func (ui *UI) runTeaIO(model tea.Model, input io.Reader, output io.Writer) error {
	_, err := tea.NewProgram(sizedModel{ui: ui, model: model}, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func joinLines(lines ...string) string { return strings.Join(lines, "\n") }
