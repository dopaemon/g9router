package tui

import "github.com/charmbracelet/lipgloss"

var palette = struct {
	Canvas   lipgloss.Color
	Surface  lipgloss.Color
	Rose     lipgloss.Color
	Pink     lipgloss.Color
	Coral    lipgloss.Color
	Green    lipgloss.Color
	Mint     lipgloss.Color
	Lavender lipgloss.Color
	Text     lipgloss.Color
	Muted    lipgloss.Color
}{
	Canvas:   lipgloss.Color("#0D0B10"),
	Surface:  lipgloss.Color("#17131C"),
	Rose:     lipgloss.Color("#F43F5E"),
	Pink:     lipgloss.Color("#EC4899"),
	Coral:    lipgloss.Color("#FB923C"),
	Green:    lipgloss.Color("#22C55E"),
	Mint:     lipgloss.Color("#2DD4BF"),
	Lavender: lipgloss.Color("#A78BFA"),
	Text:     lipgloss.Color("#F8FAFC"),
	Muted:    lipgloss.Color("#94A3B8"),
}

var styles = struct {
	Brand, Title, Subtitle, Panel, Selected, Item, Muted, Success, Warning, Error, Footer lipgloss.Style
}{
	Brand:    lipgloss.NewStyle().Bold(true).Foreground(palette.Pink),
	Title:    lipgloss.NewStyle().Bold(true).Foreground(palette.Text),
	Subtitle: lipgloss.NewStyle().Foreground(palette.Muted),
	Panel:    lipgloss.NewStyle().Background(palette.Surface).Border(lipgloss.RoundedBorder()).BorderForeground(palette.Rose).Padding(1, 2),
	Selected: lipgloss.NewStyle().Bold(true).Foreground(palette.Pink).Padding(0, 1),
	Item:     lipgloss.NewStyle().Foreground(palette.Text).Padding(0, 1),
	Muted:    lipgloss.NewStyle().Foreground(palette.Muted),
	Success:  lipgloss.NewStyle().Bold(true).Foreground(palette.Green),
	Warning:  lipgloss.NewStyle().Bold(true).Foreground(palette.Coral),
	Error:    lipgloss.NewStyle().Bold(true).Foreground(palette.Rose),
	Footer:   lipgloss.NewStyle().Foreground(palette.Muted).BorderTop(true).BorderForeground(palette.Lavender).PaddingTop(1),
}

func gradient(text string) string {
	if text == "" {
		return ""
	}
	colors := []lipgloss.Color{palette.Rose, palette.Pink, palette.Lavender, palette.Mint, palette.Green}
	out := make([]rune, 0, len([]rune(text)))
	for _, value := range text {
		out = append(out, value)
	}
	result := ""
	for index, value := range out {
		result += lipgloss.NewStyle().Foreground(colors[index*len(colors)/len(out)]).Render(string(value))
	}
	return result
}
