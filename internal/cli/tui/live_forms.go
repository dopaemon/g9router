package tui

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/charmbracelet/huh"
)

func (ui *UI) promptProviderTUI(item *provider, edit bool, input io.Reader, output io.Writer) error {
	apiType := item.APIType
	if apiType == "" {
		apiType = "openai"
	}
	result, err := ui.runTUIForm(ui.t("screen.providerForm"), []tuiField{
		{label: ui.t("form.providerID"), kind: tuiInput, value: item.ID},
		{label: ui.t("form.displayName"), kind: tuiInput, value: item.Name},
		{label: ui.t("form.baseURL"), kind: tuiInput, value: item.BaseURL},
		{label: ui.t("form.apiKey"), kind: tuiInput, value: item.APIKey, password: true},
		{label: ui.t("form.apiType"), kind: tuiSelect, options: []string{"openai", "anthropic", "gemini"}, value: apiType},
		{label: ui.t("form.finish"), kind: tuiSelect, options: []string{ui.t("common.save"), ui.t("common.back")}, value: ui.t("common.save")},
	}, input, output)
	if err != nil {
		return err
	}
	if result.values[5] == ui.t("common.back") {
		return huh.ErrUserAborted
	}
	item.ID, item.Name, item.BaseURL, item.APIKey, item.APIType = strings.TrimSpace(result.values[0]), strings.TrimSpace(result.values[1]), strings.TrimSpace(result.values[2]), result.values[3], result.values[4]
	if err := validateProviderValues(item.ID, item.BaseURL, item.APIKey); err != nil {
		return err
	}
	item.Enabled = true
	method, path := http.MethodPost, "/api/providers"
	if edit {
		method, path = http.MethodPut, "/api/providers/"+item.ID
	}
	return ui.request(method, path, item, nil)
}

func (ui *UI) promptAPIKeyTUI(input io.Reader, output io.Writer) (apiKey, error) {
	result, err := ui.runTUIForm(ui.t("keys.create"), []tuiField{
		{label: ui.t("form.apiName"), kind: tuiInput},
		{label: ui.t("form.finish"), kind: tuiSelect, options: []string{ui.t("common.save"), ui.t("common.back")}, value: ui.t("common.save")},
	}, input, output)
	if err != nil {
		return apiKey{}, err
	}
	if result.values[1] == ui.t("common.back") {
		return apiKey{}, huh.ErrUserAborted
	}
	var created apiKey
	err = ui.request(http.MethodPost, "/api/keys", map[string]string{"name": strings.TrimSpace(result.values[0])}, &created)
	return created, err
}

func (ui *UI) promptAPIKeyRenameTUI(key *apiKey, input io.Reader, output io.Writer) error {
	result, err := ui.runTUIForm(ui.t("keys.rename"), []tuiField{
		{label: ui.t("form.apiName"), kind: tuiInput, value: key.Name},
		{label: ui.t("form.finish"), kind: tuiSelect, options: []string{ui.t("common.save"), ui.t("common.back")}, value: ui.t("common.save")},
	}, input, output)
	if err != nil {
		return err
	}
	if result.values[1] == ui.t("common.back") {
		return huh.ErrUserAborted
	}
	return ui.request(http.MethodPut, "/api/keys/"+key.ID, map[string]string{"name": strings.TrimSpace(result.values[0])}, nil)
}

func (ui *UI) promptComboTUI(item *combo, edit bool, input io.Reader, output io.Writer) error {
	options, err := ui.comboModelValues()
	if err != nil {
		return err
	}
	selected := comboModels(*item)
	result, err := ui.runTUIForm(ui.t("screen.createCombo"), []tuiField{
		{label: ui.t("screen.comboName"), kind: tuiInput, value: item.Name},
		{label: ui.t("screen.modelsList"), kind: tuiMultiSelect, options: options, selected: selectedToMarks(options, selected)},
		{label: ui.t("form.finish"), kind: tuiSelect, options: []string{ui.t("common.save"), ui.t("common.back")}, value: ui.t("common.save")},
	}, input, output)
	if err != nil {
		return err
	}
	if result.values[2] == ui.t("common.back") {
		return huh.ErrUserAborted
	}
	item.Name = strings.TrimSpace(result.values[0])
	item.Models = make([]any, len(result.selected[1]))
	for index, model := range result.selected[1] {
		item.Models[index] = model
	}
	if err := validateComboValues(item.Name, comboModels(*item)); err != nil {
		return err
	}
	method, path := http.MethodPost, "/api/combos"
	if edit {
		method, path = http.MethodPut, "/api/combos/"+item.ID
	}
	return ui.request(method, path, item, nil)
}

func selectedToMarks(options, selected []string) []bool {
	marks := make([]bool, len(options))
	for index, option := range options {
		for _, current := range selected {
			marks[index] = marks[index] || option == current
		}
	}
	return marks
}

func (ui *UI) comboModelValues() ([]string, error) {
	var payload comboModelResponse
	if err := ui.request(http.MethodGet, "/api/models", nil, &payload); err != nil {
		return nil, err
	}
	values := make([]string, 0, len(payload.Models))
	for _, item := range payload.Models {
		value := item.RoutedModel
		if value == "" {
			value = item.Provider + "/" + item.Model
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("no models found")
	}
	return values, nil
}

func (ui *UI) tuiSelectKey(keys []apiKey, input io.Reader, output io.Writer) (apiKey, error) {
	if len(keys) == 0 {
		return apiKey{}, errors.New("no API keys found")
	}
	options := make([]string, len(keys))
	for index, key := range keys {
		options[index] = key.Name
	}
	selected, err := ui.tuiSelect(ui.t("form.chooseKey"), ui.t("form.chooseKey"), options, input, output)
	if err != nil {
		return apiKey{}, err
	}
	for _, key := range keys {
		if key.Name == selected {
			return key, nil
		}
	}
	return apiKey{}, errors.New("invalid API key")
}
