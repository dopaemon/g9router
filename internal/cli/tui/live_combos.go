package tui

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type comboRefreshMsg struct{}
type comboActionDoneMsg struct {
	notice string
	err    error
}

type comboLiveModel struct {
	ui     *UI
	draft  combo
	combos []combo
	cursor int
	notice string
	err    error
}

type comboModelResponse struct {
	Models []struct {
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		Name        string `json:"name"`
		RoutedModel string `json:"routedModel"`
		Alias       string `json:"alias"`
	} `json:"models"`
}

func (ui *UI) comboModelOptions() ([]huh.Option[string], error) {
	var payload comboModelResponse
	if err := ui.request(http.MethodGet, "/api/models", nil, &payload); err != nil {
		return nil, err
	}
	options := make([]huh.Option[string], 0, len(payload.Models))
	for _, item := range payload.Models {
		value := item.RoutedModel
		if value == "" {
			value = item.Provider + "/" + item.Model
		}
		label := item.Name
		if label == "" {
			label = value
		}
		if item.Alias != "" && item.Alias != item.Model {
			label += " (" + item.Alias + ")"
		}
		options = append(options, huh.NewOption(label, value))
	}
	if len(options) == 0 {
		return nil, errors.New("no models found")
	}
	return options, nil
}

func (ui *UI) liveCombos() error {
	EnableColors(ui.Out)
	model := comboLiveModel{ui: ui}
	model.refresh()
	return ui.runTea(&model)
}

func (model *comboLiveModel) Init() tea.Cmd { return comboRefresh() }

func (model *comboLiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		if model.err != nil && message.String() == "r" {
			model.err = nil
			model.refresh()
			return model, comboRefresh()
		}
		if index, err := strconv.Atoi(message.String()); err == nil && index >= 1 && index <= model.itemCount() {
			model.cursor = index - 1
			return model, model.runAction()
		}
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return model, tea.Quit
		case "up", "k":
			if model.cursor > 0 {
				model.cursor--
			}
		case "down", "j":
			if model.cursor+1 < model.itemCount() {
				model.cursor++
			}
		case "enter", " ":
			return model, model.runAction()
		case "c":
			model.cursor = 0
			return model, model.action(model.create)
		case "a":
			model.cursor = 1
			return model, model.action(model.addModel)
		case "r":
			model.cursor = 2
			return model, model.action(model.removeModel)
		case "e":
			return model, model.action(model.edit)
		case "d":
			return model, model.action(model.delete)
		}
	case comboRefreshMsg:
		model.refresh()
		return model, comboRefresh()
	case comboActionDoneMsg:
		if errors.Is(message.err, huh.ErrUserAborted) {
			message.err = nil
		}
		model.err, model.notice = message.err, message.notice
		if model.err == nil {
			model.refresh()
		}
		return model, comboRefresh()
	}
	return model, nil
}

func (model *comboLiveModel) View() string {
	if model.err != nil {
		return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.combos")) + "\n\n" + errorStyle.Render(model.ui.t("common.error")+": "+model.err.Error()) + "\n\n" + mutedStyle.Render(model.ui.t("common.retryBack")))
	}
	cards := []string{model.createCard()}
	for index, item := range model.combos {
		cards = append(cards, model.comboCard(index, item))
	}
	controls := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, model.ui.controlStyle().Render(mutedStyle.Render("↑↓/jk move")), model.ui.controlStyle().Render(mutedStyle.Render("Enter select"))),
		lipgloss.JoinHorizontal(lipgloss.Top, model.ui.controlStyle().Render(mutedStyle.Render("c create  a add model")), model.ui.controlStyle().Render(mutedStyle.Render("r remove model"))),
		lipgloss.JoinHorizontal(lipgloss.Top, model.ui.controlStyle().Render(mutedStyle.Render("e edit  d delete")), model.ui.controlStyle().Render(mutedStyle.Render("q back"))),
	)
	controlCard := model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("common.controls")) + "\n" + controls)
	content := lipgloss.JoinVertical(lipgloss.Center, cards...)
	if model.notice != "" {
		content += "\n" + successStyle.Render(model.notice)
	}
	return model.ui.outerStyle().Render(cardTitleStyle.Render(model.ui.t("menu.combos")) + "\n\n" + content + "\n\n" + controlCard)
}

func (model *comboLiveModel) createCard() string {
	models := strings.Join(comboModels(model.draft), ", ")
	rows := []string{
		comboActionItem(0, model.ui.t("screen.createCombo"), model.cursor == 0),
		model.ui.t("screen.comboName") + ": " + valueOrDash(model.draft.Name),
		model.ui.t("screen.modelsList") + ": " + valueOrDash(models),
		comboActionItem(1, model.ui.t("screen.addModel"), model.cursor == 1),
		comboActionItem(2, model.ui.t("screen.removeModel"), model.cursor == 2),
	}
	return model.ui.innerStyle().Render(cardTitleStyle.Render(model.ui.t("screen.createCombo")) + "\n" + strings.Join(rows, "\n"))
}

func (model *comboLiveModel) comboCard(cardIndex int, item combo) string {
	editCursor := 3 + cardIndex*2
	deleteCursor := editCursor + 1
	rows := []string{
		model.ui.t("screen.comboName") + ": " + item.Name,
		model.ui.t("screen.modelsList") + ": " + valueOrDash(strings.Join(comboModels(item), ", ")),
		comboActionItem(editCursor, model.ui.t("screen.edit"), model.cursor == editCursor),
		comboActionItem(deleteCursor, model.ui.t("screen.delete"), model.cursor == deleteCursor),
	}
	return model.ui.innerStyle().Render(cardTitleStyle.Render("Combo: "+item.Name) + "\n" + strings.Join(rows, "\n"))
}

func comboActionItem(index int, label string, selected bool) string {
	text := strconv.Itoa(index+1) + "  " + label
	if selected {
		return focusStyle.Render(text)
	}
	return controlsStyle.Render(text)
}

func (model *comboLiveModel) itemCount() int { return 3 + len(model.combos)*2 }

func (model *comboLiveModel) runAction() tea.Cmd {
	switch {
	case model.cursor == 0:
		return model.action(model.create)
	case model.cursor == 1:
		return model.action(model.addModel)
	case model.cursor == 2:
		return model.action(model.removeModel)
	case (model.cursor-3)%2 == 0:
		return model.action(model.edit)
	default:
		return model.action(model.delete)
	}
}

func (model *comboLiveModel) action(run func(io.Reader, io.Writer) (string, error)) tea.Cmd {
	var notice string
	return tea.Exec(&endpointExecCommand{run: func(input io.Reader, output io.Writer) error {
		var err error
		notice, err = run(input, output)
		return err
	}}, func(err error) tea.Msg { return comboActionDoneMsg{notice: notice, err: err} })
}

func (model *comboLiveModel) create(input io.Reader, output io.Writer) (string, error) {
	if strings.TrimSpace(model.draft.Name) == "" {
		name, err := model.comboName(input, output)
		if err != nil {
			return "", err
		}
		model.draft.Name = name
	}
	if err := model.ui.request(http.MethodPost, "/api/combos", model.draft, nil); err != nil {
		return "", err
	}
	model.draft = combo{}
	return "Combo created", nil
}

func (model *comboLiveModel) comboName(input io.Reader, output io.Writer) (string, error) {
	result, err := model.ui.runTUIForm(model.ui.t("screen.createCombo"), []tuiField{
		{label: model.ui.t("screen.comboName"), kind: tuiInput, value: model.draft.Name},
	}, input, output)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.values[0]), nil
}

func (model *comboLiveModel) addModel(input io.Reader, output io.Writer) (string, error) {
	options, err := model.ui.comboModelValues()
	if err != nil {
		return "", err
	}
	selected, err := model.ui.tuiSelect(model.ui.t("screen.addModel"), model.ui.t("screen.modelsList"), options, input, output)
	if err != nil {
		return "", err
	}
	model.draft.Models = append(model.draft.Models, selected)
	return "Model added", nil
}

func (model *comboLiveModel) removeModel(input io.Reader, output io.Writer) (string, error) {
	models := comboModels(model.draft)
	if len(models) == 0 {
		return "No models to remove", nil
	}
	selected, err := model.ui.tuiSelect(model.ui.t("screen.removeModel"), model.ui.t("screen.modelsList"), models, input, output)
	if err != nil {
		return "", err
	}
	filtered := model.draft.Models[:0]
	removed := false
	for _, value := range model.draft.Models {
		if text, ok := value.(string); ok && text == selected && !removed {
			removed = true
			continue
		}
		filtered = append(filtered, value)
	}
	model.draft.Models = filtered
	return "Model removed", nil
}

func (model *comboLiveModel) edit(input io.Reader, output io.Writer) (string, error) {
	index := (model.cursor - 3) / 2
	if index < 0 || index >= len(model.combos) {
		return "", nil
	}
	item := model.combos[index]
	if err := model.ui.promptComboTUI(&item, true, input, output); err != nil {
		return "", err
	}
	return "Combo updated", nil
}

func (model *comboLiveModel) delete(input io.Reader, output io.Writer) (string, error) {
	index := (model.cursor - 3) / 2
	if index < 0 || index >= len(model.combos) {
		return "", nil
	}
	item := model.combos[index]
	ok, err := model.ui.tuiConfirm("Delete combo "+item.Name+"?", input, output)
	if err != nil || !ok {
		return "", err
	}
	return "Combo deleted", model.ui.request(http.MethodDelete, "/api/combos/"+url.PathEscape(item.ID), nil, nil)
}

func (model *comboLiveModel) refresh() {
	var payload struct {
		Combos []combo `json:"combos"`
	}
	if err := model.ui.request(http.MethodGet, "/api/combos", nil, &payload); err != nil {
		model.err = err
		return
	}
	model.combos, model.err = payload.Combos, nil
	if model.cursor >= model.itemCount() {
		model.cursor = 0
	}
}

func comboModels(item combo) []string {
	models := make([]string, 0, len(item.Models))
	for _, value := range item.Models {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			models = append(models, text)
		}
	}
	return models
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func comboRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return comboRefreshMsg{} })
}
