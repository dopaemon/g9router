package tui

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var consoleStyles = struct {
	Info, Success, Error, Muted lipgloss.Style
}{
	Info:    lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9")),
	Success: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#86EFAC")),
	Error:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FDA4AF")),
	Muted:   lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")),
}

func Info(message string) string    { return consoleStyles.Info.Render("• " + message) }
func Success(message string) string { return consoleStyles.Success.Render("✓ " + message) }
func Error(message string) string   { return consoleStyles.Error.Render("✗ " + message) }
func Muted(message string) string   { return consoleStyles.Muted.Render(message) }

func EnableColors(output io.Writer) {
	file, ok := output.(*os.File)
	if ok && IsTerminal(file) {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
}
