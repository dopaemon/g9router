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
	resized := false
	if size, ok := message.(tea.WindowSizeMsg); ok {
		resized = model.ui.width != size.Width || model.ui.height != size.Height
		model.ui.width, model.ui.height = size.Width, size.Height
		model.ui.viewClipped = false
	}
	updated, command := model.model.Update(message)
	model.model = updated
	if resized {
		command = tea.Batch(command, func() tea.Msg { return tea.ClearScreen() })
	}
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
	return max(1, min(width, 120))
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

func (ui *UI) responsiveColumns(total, minimum int) int {
	if total < 1 || minimum < 1 {
		return 1
	}
	return max(1, min(total, (ui.innerWidth()+2)/(minimum+2)))
}

func (ui *UI) responsiveCardWidth(total, minimum int) int {
	columns := ui.responsiveColumns(total, minimum)
	return max(1, (ui.innerWidth()-2*(columns-1)-2*columns)/columns)
}

func (ui *UI) joinResponsiveCards(cards []string, columns int) string {
	if len(cards) == 0 {
		return ""
	}
	rows := make([]string, 0, (len(cards)+columns-1)/columns)
	for index := 0; index < len(cards); index += columns {
		end := min(index+columns, len(cards))
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards[index:end]...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
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
	return ui.controlColumnsSelected(-1, items...)
}

func (ui *UI) controlColumnsSelected(selected int, items ...string) string {
	if len(items) == 0 {
		return ""
	}
	if ui.compact() {
		rows := make([]string, 0, len(items))
		for index, item := range items {
			style := mutedStyle
			if index == selected {
				style = focusStyle
			}
			rows = append(rows, ui.controlStyle().Render(style.Render(item)))
		}
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}
	rows := make([]string, 0, (len(items)+1)/2)
	for index := 0; index < len(items); index += 2 {
		leftStyle := mutedStyle
		if index == selected {
			leftStyle = focusStyle
		}
		left := ui.controlStyle().Render(leftStyle.Render(items[index]))
		right := ""
		if index+1 < len(items) {
			rightStyle := mutedStyle
			if index+1 == selected {
				rightStyle = focusStyle
			}
			right = ui.controlStyle().Render(rightStyle.Render(items[index+1]))
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
