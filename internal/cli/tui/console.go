package tui

import "github.com/charmbracelet/lipgloss"

var consoleStyles = struct {
	Info, Success, Error, Muted lipgloss.Style
}{
	Info:    lipgloss.NewStyle().Foreground(palette.Lavender),
	Success: lipgloss.NewStyle().Bold(true).Foreground(palette.Green),
	Error:   lipgloss.NewStyle().Bold(true).Foreground(palette.Rose),
	Muted:   lipgloss.NewStyle().Foreground(palette.Muted),
}

func Info(message string) string    { return consoleStyles.Info.Render("• " + message) }
func Success(message string) string { return consoleStyles.Success.Render("✓ " + message) }
func Error(message string) string   { return consoleStyles.Error.Render("✗ " + message) }
func Muted(message string) string   { return consoleStyles.Muted.Render(message) }
