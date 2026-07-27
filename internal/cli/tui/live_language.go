package tui

import (
	"strconv"

	"g9router/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type languageModel struct {
	ui     *UI
	cursor int
}

func (ui *UI) liveLanguage() error {
	model := languageModel{ui: ui}
	if ui.Locale == i18n.Vietnamese {
		model.cursor = 1
	}
	return ui.runTea(&model)
}

func (model *languageModel) Init() tea.Cmd { return nil }

func (model *languageModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		if index, err := strconv.Atoi(key.String()); err == nil && index >= 1 && index <= 2 {
			model.cursor = index - 1
			return model, model.selectLanguage()
		}
		switch key.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "left", "h", "up", "k":
			model.cursor = 0
		case "right", "l", "down", "j":
			model.cursor = 1
		case "enter", " ":
			return model, model.selectLanguage()
		}
	}
	return model, nil
}

func (model *languageModel) selectLanguage() tea.Cmd {
	if model.cursor == 1 {
		model.ui.Locale = i18n.Vietnamese
	} else {
		model.ui.Locale = i18n.English
	}
	return tea.Quit
}

func (model *languageModel) View() string {
	english := languageCard(0, "English", model.cursor == 0)
	vietnamese := languageCard(1, "Tiếng Việt", model.cursor == 1)
	controls := innerCardStyle.Render(cardTitleStyle.Render("Controls") + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("←→/hl switch")),
		lipgloss.NewStyle().Width(30).Render(mutedStyle.Render("Enter select  q back")),
	))
	banner := lipgloss.NewStyle().Width(78).Align(lipgloss.Center).Render(gradientText(cliBanner))
	return outerCardStyle.Render(banner + "\n\n" + cardTitleStyle.Render(model.ui.t("language.title")) + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, english, vietnamese) + "\n\n" + controls)
}

func languageCard(index int, label string, selected bool) string {
	text := strconv.Itoa(index+1) + "  " + label
	if selected {
		text = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B1020")).Background(lipgloss.Color("#67E8F9")).Padding(0, 1).Render(text)
	} else {
		text = controlsStyle.Render(text)
	}
	return innerCardStyle.Width(31).Render(text)
}
