package tui

import (
	"bufio"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type creditsModel struct{ ui *UI }

func (ui *UI) liveCredits() error { return ui.runTea(&creditsModel{ui: ui}) }

func (ui *UI) accessibleCredits(reader *bufio.Reader) error {
	fmt.Fprintln(ui.Out, "\n"+ui.t("credits.title"))
	fmt.Fprintln(ui.Out, ui.t("credits.app"))
	fmt.Fprintln(ui.Out, ui.t("credits.description"))
	fmt.Fprintln(ui.Out, ui.t("credits.builtWith"))
	fmt.Fprintln(ui.Out, ui.t("credits.inspired"))
	_, err := ui.readChoice(reader, ui.t("common.controls"), []string{ui.t("common.back")})
	return err
}

func (model *creditsModel) Init() tea.Cmd { return nil }

func (model *creditsModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "esc", "ctrl+c", "enter", " ":
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model *creditsModel) View() string {
	body := strings.Join([]string{
		cardTitleStyle.Render(model.ui.t("credits.app")),
		model.ui.t("credits.description"),
		model.ui.t("credits.builtWith"),
		model.ui.t("credits.inspired"),
	}, "\n")
	content := model.ui.innerStyle().Render(body)
	controls := model.ui.controlCard(model.ui.t("common.controls"), "q "+model.ui.t("common.back"))
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("credits.title")) + "\n\n" + content + "\n\n" + controls)
}
