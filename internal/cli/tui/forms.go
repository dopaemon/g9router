package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"g9router/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

var errUserAborted = errors.New("user aborted")

type tuiFieldKind int

const (
	tuiInput tuiFieldKind = iota
	tuiSelect
	tuiMultiSelect
	tuiConfirm
)

type tuiField struct {
	label    string
	kind     tuiFieldKind
	value    string
	password bool
	options  []string
	choice   int
	selected []bool
	confirm  bool
}

type tuiForm struct {
	ui     *UI
	title  string
	fields []tuiField
	cursor int
	err    error
}

type tuiFormResult struct {
	values   []string
	selected [][]string
	confirms []bool
}

func (ui *UI) runTUIForm(title string, fields []tuiField, input io.Reader, output io.Writer) (tuiFormResult, error) {
	model := &tuiForm{ui: ui, title: title, fields: fields}
	for index := range model.fields {
		if model.fields[index].kind == tuiMultiSelect && len(model.fields[index].selected) == 0 {
			model.fields[index].selected = make([]bool, len(model.fields[index].options))
		}
		for option, value := range model.fields[index].options {
			if value == model.fields[index].value {
				model.fields[index].choice = option
			}
		}
	}
	if err := ui.runTeaIO(model, input, output); err != nil {
		return tuiFormResult{}, err
	}
	if model.err != nil {
		return tuiFormResult{}, model.err
	}
	result := tuiFormResult{}
	for _, field := range model.fields {
		result.values = append(result.values, field.value)
		result.confirms = append(result.confirms, field.confirm)
		selected := make([]string, 0, len(field.options))
		for index, value := range field.options {
			if field.kind == tuiMultiSelect && index < len(field.selected) && field.selected[index] {
				selected = append(selected, value)
			}
		}
		result.selected = append(result.selected, selected)
	}
	return result, nil
}

func (model *tuiForm) Init() tea.Cmd { return nil }

func (model *tuiForm) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	model.err = nil
	field := &model.fields[model.cursor]
	if field.kind == tuiInput {
		switch key.String() {
		case "ctrl+c", "esc":
			model.err = errUserAborted
			return model, tea.Quit
		case "tab":
			model.next()
		case "shift+tab":
			model.previous()
		case "up":
			model.previous()
		case "down":
			model.next()
		case "backspace", "ctrl+h":
			value := []rune(field.value)
			if len(value) > 0 {
				field.value = string(value[:len(value)-1])
			}
		case "ctrl+u":
			field.value = ""
		case "enter", "ctrl+j", "ctrl+m":
			if model.cursor+1 == len(model.fields) {
				if err := model.validate(); err != nil {
					model.err = err
					return model, nil
				}
				return model, tea.Quit
			}
			model.next()
		default:
			if len(key.Runes) > 0 {
				field.value += string(key.Runes)
			}
		}
		return model, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		model.err = errUserAborted
		return model, tea.Quit
	case "q":
		model.err = errUserAborted
		return model, tea.Quit
	case "tab":
		model.next()
	case "shift+tab":
		model.previous()
	case "down", "j":
		if field.kind == tuiSelect || field.kind == tuiMultiSelect || field.kind == tuiConfirm {
			model.nextOption(field)
		} else {
			model.next()
		}
	case "up", "k":
		if field.kind == tuiSelect || field.kind == tuiMultiSelect || field.kind == tuiConfirm {
			model.previousOption(field)
		} else {
			model.previous()
		}
	case "left", "h":
		model.previousOption(field)
	case "right", "l":
		model.nextOption(field)
	case "space", " ":
		model.toggle(field)
	case "enter", "ctrl+j", "ctrl+m":
		if model.cursor+1 == len(model.fields) {
			if err := model.validate(); err != nil {
				model.err = err
				return model, nil
			}
			return model, tea.Quit
		}
		model.next()
	default:
		if len(key.Runes) == 1 && key.Runes[0] >= '1' && key.Runes[0] <= '9' {
			option := int(key.Runes[0] - '1')
			if (field.kind == tuiSelect || field.kind == tuiMultiSelect) && option < len(field.options) {
				field.choice = option
				if field.kind == tuiSelect {
					field.value = field.options[option]
				}
			}
		}
	}
	return model, nil
}

func (model *tuiForm) next() {
	model.cursor = cycleIndex(model.cursor, len(model.fields), 1)
}

func (model *tuiForm) previous() {
	model.cursor = cycleIndex(model.cursor, len(model.fields), -1)
}

func (model *tuiForm) nextOption(field *tuiField) {
	if field.kind == tuiConfirm {
		field.confirm = true
		return
	}
	if field.kind == tuiSelect && field.choice+1 < len(field.options) {
		field.choice++
		field.value = field.options[field.choice]
	}
	if field.kind == tuiMultiSelect && field.choice+1 < len(field.options) {
		field.choice++
	}
}

func (model *tuiForm) previousOption(field *tuiField) {
	if field.kind == tuiConfirm {
		field.confirm = false
		return
	}
	if field.kind == tuiSelect && field.choice > 0 {
		field.choice--
		field.value = field.options[field.choice]
	}
	if field.kind == tuiMultiSelect && field.choice > 0 {
		field.choice--
	}
}

func (model *tuiForm) toggle(field *tuiField) {
	switch field.kind {
	case tuiConfirm:
		field.confirm = !field.confirm
	case tuiMultiSelect:
		if field.choice < len(field.selected) {
			field.selected[field.choice] = !field.selected[field.choice]
		}
		if field.choice+1 < len(field.options) {
			field.choice++
		}
	}
}

func (model *tuiForm) validate() error {
	for _, field := range model.fields {
		if field.kind == tuiInput && ((!field.password && strings.TrimSpace(field.value) == "") || (field.password && field.value == "")) {
			return errors.New(fmt.Sprintf(model.ui.t("form.required"), field.label))
		}
		if field.kind == tuiMultiSelect && len(field.options) > 0 {
			selected := false
			for _, value := range field.selected {
				selected = selected || value
			}
			if !selected {
				return errors.New(model.ui.t("form.selectOption"))
			}
		}
	}
	return nil
}

func (model *tuiForm) View() string {
	rows := make([]string, 0, len(model.fields))
	for index := range model.fields {
		field := &model.fields[index]
		if field.kind == tuiMultiSelect || field.kind == tuiSelect || field.kind == tuiConfirm {
			if index != model.cursor {
				rows = append(rows, controlsStyle.Render(field.label+": "+model.fieldValue(field)))
				continue
			}
			menu := model.optionRows(field)
			rows = append(rows, cardTitleStyle.Render(field.label+":")+"\n"+multiSelectStyle.Width(model.ui.innerWidth()).Render(menu))
			continue
		}
		label := field.label
		value := model.fieldValue(field)
		line := label + ": " + value
		if index == model.cursor {
			line = focusStyle.Render(line)
		} else {
			line = controlsStyle.Render(line)
		}
		rows = append(rows, line)
	}
	content := cardTitleStyle.Render(model.title) + "\n\n" + strings.Join(rows, "\n") + "\n\n" + mutedStyle.Render(model.ui.t("form.controls"))
	if model.ui.height > 0 && model.ui.height < 18 {
		content = cardTitleStyle.Render(model.title) + "\n" + strings.Join(rows, "\n") + "\n" + mutedStyle.Render(model.ui.t("form.controls"))
	}
	if model.err != nil {
		content += "\n\n" + errorStyle.Render(model.err.Error())
	}
	return model.ui.outerStyle().Render(content)
}

func (model *tuiForm) optionRows(field *tuiField) string {
	options := field.options
	selected := field.choice
	if field.kind == tuiConfirm {
		options = []string{i18n.T(model.ui.Locale, "common.no"), i18n.T(model.ui.Locale, "common.yes")}
		if field.confirm {
			selected = 1
		} else {
			selected = 0
		}
	}
	start, end := viewportWindow(selected, len(options), model.ui.viewportHeight(9, 10))
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		marker := " "
		if index == selected {
			marker = ">"
		}
		line := fmt.Sprintf("%s %d  %s", marker, index+1, options[index])
		rows = append(rows, truncateText(line, model.ui.innerWidth()-2))
	}
	return strings.Join(rows, "\n")
}

func (model *tuiForm) multiSelectRows(field *tuiField) string {
	start, end := model.multiSelectWindow(field)
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		mark := " "
		if index < len(field.selected) && field.selected[index] {
			mark = "x"
		}
		line := "  [" + mark + "] " + field.options[index]
		if index == field.choice {
			line = multiCursorStyle.Render("> [" + mark + "] " + field.options[index])
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (model *tuiForm) multiSelectWindow(field *tuiField) (int, int) {
	start, end := 0, len(field.options)
	visible := model.ui.viewportHeight(9, 10)
	if end <= visible {
		return start, end
	}
	return viewportWindow(field.choice, len(field.options), visible)
}

func (model *tuiForm) fieldValue(field *tuiField) string {
	switch field.kind {
	case tuiInput:
		if field.password && field.value != "" {
			return strings.Repeat("•", len([]rune(field.value)))
		}
		return field.value
	case tuiSelect:
		return field.value
	case tuiMultiSelect:
		start, end := model.multiSelectWindow(field)
		values := make([]string, 0, end-start)
		for index := start; index < end; index++ {
			mark := " "
			if index < len(field.selected) && field.selected[index] {
				mark = "x"
			}
			cursor := " "
			if index == field.choice {
				cursor = ">"
			}
			values = append(values, cursor+" ["+mark+"] "+field.options[index])
		}
		return strings.Join(values, "\n")
	case tuiConfirm:
		if field.confirm {
			return i18n.T(model.ui.Locale, "common.yes")
		}
		return i18n.T(model.ui.Locale, "common.no")
	default:
		return ""
	}
}

func (ui *UI) tuiConfirm(title string, input io.Reader, output io.Writer) (bool, error) {
	result, err := ui.runTUIForm(title, []tuiField{{label: ui.t("common.confirm"), kind: tuiConfirm, confirm: true}}, input, output)
	if err != nil {
		return false, err
	}
	return result.confirms[0], nil
}

func (ui *UI) tuiInput(title, label, value string, password bool, input io.Reader, output io.Writer) (string, error) {
	result, err := ui.runTUIForm(title, []tuiField{{label: label, kind: tuiInput, value: value, password: password}}, input, output)
	if err != nil {
		return "", err
	}
	return result.values[0], nil
}

func (ui *UI) tuiSelect(title, label string, options []string, input io.Reader, output io.Writer) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no choices available")
	}
	result, err := ui.runTUIForm(title, []tuiField{{label: label, kind: tuiSelect, options: options, value: options[0]}}, input, output)
	if err != nil {
		return "", err
	}
	return result.values[0], nil
}

func (ui *UI) tuiMultiSelect(title, label string, options, selected []string, input io.Reader, output io.Writer) ([]string, error) {
	marks := make([]bool, len(options))
	for index, option := range options {
		for _, current := range selected {
			if option == current {
				marks[index] = true
			}
		}
	}
	result, err := ui.runTUIForm(title, []tuiField{{label: label, kind: tuiMultiSelect, options: options, selected: marks}}, input, output)
	if err != nil {
		return nil, err
	}
	return result.selected[0], nil
}
