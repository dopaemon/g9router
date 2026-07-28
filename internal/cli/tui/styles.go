package tui

import (
	"fmt"

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

func statusText(enabled bool) string {
	return statusTextLocale(enabled, "en")
}

func statusTextLocale(enabled bool, locale string) string {
	color := "#FB7185"
	if enabled {
		color = "#4ADE80"
	}
	label := "common.off"
	if enabled {
		label = "common.on"
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(i18n.T(locale, label))
}

func gradientText(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	result := ""
	for index, value := range runes {
		ratio := float64(index) / float64(len(runes)-1)
		if len(runes) == 1 {
			ratio = 0
		}
		red := int(34 + 217*ratio)
		green := int(211 - 131*ratio)
		blue := int(238 + 17*ratio)
		result += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", red, green, blue))).Render(string(value))
	}
	return result
}
