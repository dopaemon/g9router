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
		width = 76
	}
	if width > 78 {
		width = 78
	}
	if width < 20 {
		width = 20
	}
	return width
}

func (ui *UI) innerWidth() int {
	width := ui.cardWidth() - 8
	if width < 12 {
		return 12
	}
	return width
}

func (ui *UI) columnWidth(columns int) int {
	width := (ui.innerWidth() - columns + 1) / columns
	return width
}

func (ui *UI) outerStyle() lipgloss.Style {
	return outerCardStyle.Width(ui.cardWidth())
}

func (ui *UI) innerStyle(width ...int) lipgloss.Style {
	cardWidth := ui.innerWidth()
	if len(width) > 0 && width[0] < cardWidth {
		cardWidth = width[0]
	}
	return innerCardStyle.Width(cardWidth)
}

func (ui *UI) controlStyle() lipgloss.Style {
	return lipgloss.NewStyle().Width(ui.columnWidth(2))
}

func (ui *UI) runTeaIO(model tea.Model, input io.Reader, output io.Writer) error {
	_, err := tea.NewProgram(sizedModel{ui: ui, model: model}, tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func joinLines(lines ...string) string { return strings.Join(lines, "\n") }
