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
	err           error
	cardsTopValue int
	cardRegions   []tuiRegion
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
	if model.ui.viewClipped {
		if key, ok := message.(tea.KeyMsg); ok && (key.String() == "q" || key.String() == "esc" || key.String() == "ctrl+c") {
			return model, tea.Quit
		}
		if _, ok := message.(tea.MouseMsg); ok {
			return model, nil
		}
	}
	if mouse, ok := message.(tea.MouseMsg); ok && (mouse.Action == tea.MouseActionPress || mouse.Action == tea.MouseActionRelease) && mouse.Button == tea.MouseButtonLeft {
		for index, region := range model.cardRegions {
			if region.contains(mouse.X, mouse.Y) {
				model.cursor = index
				return model, model.selectLanguage()
			}
		}
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
	locale := i18n.English
	if model.cursor == 1 {
		locale = i18n.Vietnamese
	}
	if err := model.ui.saveLocale(locale); err != nil {
		model.err = err
		return nil
	}
	model.ui.Locale = locale
	return tea.Quit
}

func (model *languageModel) View() string {
	if model.err != nil {
		return model.ui.errorView(model.ui.t("language.title"), model.err)
	}
	columns := model.ui.responsiveColumns(2, 24)
	cardWidth := model.ui.responsiveCardWidth(2, 24)
	english := languageCard(cardWidth, 0, model.ui.t("language.english"), model.cursor == 0)
	vietnamese := languageCard(cardWidth, 1, model.ui.t("language.vietnamese"), model.cursor == 1)
	model.cardRegions = nil
	cardTop := 2 + lipgloss.Height(cardTitleStyle.Render(model.ui.t("language.title"))) + 2
	cardLeft := 1 + 2
	cardHeight := lipgloss.Height(english)
	for index := range []string{"english", "vietnamese"} {
		row, column := index/columns, index%columns
		model.cardRegions = append(model.cardRegions, tuiRegion{
			left: cardLeft + column*cardWidth, top: cardTop + row*cardHeight, width: cardWidth, height: cardHeight,
		})
	}
	controls := model.ui.controlCard(model.ui.t("common.controls"), model.ui.controlColumns(
		model.ui.t("controls.languageSwitch"), model.ui.t("controls.languageSelect"), model.ui.t("controls.languageBack"),
	))
	model.cardsTopValue = cardTop
	cards := model.ui.joinResponsiveCards([]string{english, vietnamese}, columns)
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("language.title")) + "\n\n" + cards + "\n\n" + controls)
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
