package tui

import (
	"strconv"

	"g9router/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type languageModel struct {
	ui            *UI
	cursor        int
	cardsTopValue int
}

func (model *languageModel) cardsTop() int { return model.cardsTopValue }

func (ui *UI) liveLanguage() error {
	model := languageModel{ui: ui}
	if ui.Locale == i18n.Vietnamese {
		model.cursor = 1
	}
	return ui.runTea(&model)
}

func (model *languageModel) Init() tea.Cmd { return nil }

func (model *languageModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if mouse, ok := message.(tea.MouseMsg); ok && (mouse.Action == tea.MouseActionPress || mouse.Action == tea.MouseActionRelease) && mouse.Button == tea.MouseButtonLeft && mouse.Y >= model.cardsTop() && mouse.Y < model.cardsTop()+6 {
		if mouse.X < model.ui.columnWidth(2) {
			model.cursor = 0
		} else {
			model.cursor = 1
		}
		return model, model.selectLanguage()
	}
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
	english := languageCard(model.ui.columnWidth(2), 0, model.ui.t("language.english"), model.cursor == 0)
	vietnamese := languageCard(model.ui.columnWidth(2), 1, model.ui.t("language.vietnamese"), model.cursor == 1)
	controls := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top,
		model.ui.controlStyle().Render(mutedStyle.Render("←→/hl switch")),
		model.ui.controlStyle().Render(mutedStyle.Render("Enter select  q back")),
	))
	model.cardsTopValue = 2 + lipgloss.Height(cardTitleStyle.Render(model.ui.t("language.title"))) + 2 + 2
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("language.title")) + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, english, vietnamese) + "\n\n" + controls)
}

func languageCard(width, index int, label string, selected bool) string {
	text := strconv.Itoa(index+1) + "  " + label
	if selected {
		text = focusStyle.Render(text)
	} else {
		text = controlsStyle.Render(text)
	}
	return innerCardStyle.Width(width).Render(text)
}
