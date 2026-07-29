package tui

import (
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type sizedModel struct {
	ui    *UI
	model tea.Model
}

type tuiRegion struct {
	left   int
	top    int
	width  int
	height int
}

func (region tuiRegion) contains(x, y int) bool {
	return x >= region.left && x < region.left+region.width && y >= region.top && y < region.top+region.height
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

func (model sizedModel) View() string { return model.ui.fitView(model.model.View()) }

func (ui *UI) cardWidth() int {
	width := ui.width - 4
	if width <= 0 {
		if ui.width > 0 {
			return 1
		}
		return 76
	}
	return max(1, min(width, 78))
}

func (ui *UI) innerWidth() int {
	width := ui.cardWidth() - 8
	if width < 1 {
		return 1
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

func (ui *UI) viewportHeight(reserved, fallback int) int {
	if ui.height <= 0 {
		return fallback
	}
	return max(1, ui.height-reserved)
}

func (ui *UI) fitView(view string) string {
	ui.viewClipped = false
	if ui.height <= 0 {
		if ui.width <= 0 {
			return view
		}
		lines := strings.Split(view, "\n")
		for index := range lines {
			lines[index] = ansi.Truncate(lines[index], ui.width, "")
		}
		return strings.Join(lines, "\n")
	}
	lines := strings.Split(view, "\n")
	ui.viewClipped = ui.height > 0 && len(lines) > ui.height
	if ui.width > 0 {
		for index := range lines {
			lines[index] = ansi.Truncate(lines[index], ui.width, "")
		}
	}
	if len(lines) <= ui.height {
		return strings.Join(lines, "\n")
	}
	if ui.height == 1 {
		return lines[0]
	}
	keep := ui.height - 1
	start := len(lines) - keep
	return strings.Join(append([]string{lines[0]}, lines[start:]...), "\n")
}

func viewportWindow(cursor, total, visible int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	visible = max(1, min(visible, total))
	start := cursor - visible + 1
	if start < 0 {
		start = 0
	}
	if start+visible > total {
		start = total - visible
	}
	return start, start + visible
}

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

func (ui *UI) controlCard(title, body string) string {
	return ui.innerStyle().Padding(0, 2).Render(cardTitleStyle.Render(title) + "\n" + body)
}

func (ui *UI) controlColumns(items ...string) string {
	if len(items) == 0 {
		return ""
	}
	if ui.compact() {
		rows := make([]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, ui.controlStyle().Render(mutedStyle.Render(item)))
		}
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}
	rows := make([]string, 0, (len(items)+1)/2)
	for index := 0; index < len(items); index += 2 {
		left := ui.controlStyle().Render(mutedStyle.Render(items[index]))
		right := ""
		if index+1 < len(items) {
			right = ui.controlStyle().Render(mutedStyle.Render(items[index+1]))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, left, right))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (ui *UI) runTeaIO(model tea.Model, input io.Reader, output io.Writer) error {
	options := []tea.ProgramOption{tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen()}
	if !accessibleMode(input) {
		options = append(options, tea.WithMouseCellMotion())
	}
	_, err := tea.NewProgram(sizedModel{ui: ui, model: model}, options...).Run()
	return err
}

func joinLines(lines ...string) string { return strings.Join(lines, "\n") }
