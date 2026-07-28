package tui

import (
	"g9router/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

var outerCardStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7C3AED")).Padding(1, 2).Align(lipgloss.Center)
var innerCardStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#A855F7")).Padding(1, 2)
var cardTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F0ABFC"))
var endpointLabelStyle = lipgloss.NewStyle().Width(12).Foreground(lipgloss.Color("#CBD5E1"))
var controlsStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
var mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
var focusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1)
var errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FB7185"))
var successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
var multiSelectStyle = lipgloss.NewStyle().Background(lipgloss.Color("#000000")).Padding(0, 1)
var multiCursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F472B6"))

func statusTextLocale(enabled bool, locale string) string {
	color, marker, label := "#FB7185", "✗", "common.off"
	if enabled {
		color, marker, label = "#4ADE80", "✓", "common.on"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(marker + " " + i18n.T(locale, label))
}
