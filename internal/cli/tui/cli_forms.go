package tui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"g9router/internal/i18n"
)

func (ui *UI) useAccessible(input io.Reader) bool {
	return !ui.forceTea && accessibleMode(input)
}

func accessibleMode(input io.Reader) bool {
	if os.Getenv("G9ROUTER_ACCESSIBLE") == "1" {
		return true
	}
	file, ok := input.(*os.File)
	return !ok || !IsTerminal(file)
}

func (ui *UI) huhWidth() int {
	if ui.width <= 0 {
		return 72
	}
	return max(20, min(ui.width-4, 72))
}

func (ui *UI) promptProvider(item *provider, edit bool) error {
	return ui.promptProviderIO(item, edit, ui.In, ui.Out)
}

func (ui *UI) promptProviderIO(item *provider, edit bool, input io.Reader, output io.Writer) error {
	if item.APIType == "" {
		item.APIType = "openai"
	}
	result, err := ui.runTUIForm(ui.t("screen.providerForm"), []tuiField{
		{label: ui.t("form.providerID"), kind: tuiInput, value: item.ID},
		{label: ui.t("form.displayName"), kind: tuiInput, value: item.Name},
		{label: ui.t("form.baseURL"), kind: tuiInput, value: item.BaseURL},
		{label: ui.t("form.apiKey"), kind: tuiInput, value: item.APIKey, password: true},
		{label: ui.t("form.finish"), kind: tuiSelect, options: []string{ui.t("common.save"), ui.t("common.back")}, value: ui.t("common.save")},
	}, input, output)
	if err != nil {
		return err
	}
	if result.values[4] == ui.t("common.back") {
		return errUserAborted
	}
	item.ID, item.Name, item.BaseURL, item.APIKey = result.values[0], result.values[1], result.values[2], result.values[3]
	return ui.saveProvider(item, edit)
}

func (ui *UI) promptAPIKey() (apiKey, error) {
	return promptAPIKeyIO(ui, ui.In, ui.Out, ui.request, ui.Locale)
}

func promptAPIKeyIO(ui *UI, input io.Reader, output io.Writer, request func(string, string, any, any) error, locale string) (apiKey, error) {
	result, err := ui.runTUIForm(i18nText(locale, "keys.create"), []tuiField{
		{label: i18nText(locale, "form.apiName"), kind: tuiInput},
		{label: i18nText(locale, "form.finish"), kind: tuiSelect, options: []string{i18nText(locale, "common.save"), i18nText(locale, "common.back")}, value: i18nText(locale, "common.save")},
	}, input, output)
	if err != nil {
		return apiKey{}, err
	}
	if result.values[1] == i18nText(locale, "common.back") {
		return apiKey{}, errUserAborted
	}
	if err := validateRequiredLocale(locale, i18nText(locale, "form.apiName"), result.values[0]); err != nil {
		return apiKey{}, err
	}
	var created apiKey
	err = request(http.MethodPost, "/api/keys", map[string]string{"name": strings.TrimSpace(result.values[0])}, &created)
	return created, err
}

func (ui *UI) promptAPIKeyRename(key *apiKey) error {
	return ui.promptAPIKeyRenameIO(key, ui.In, ui.Out, ui.request, ui.Locale)
}

func (ui *UI) promptAPIKeyRenameIO(key *apiKey, input io.Reader, output io.Writer, request func(string, string, any, any) error, locale string) error {
	result, err := ui.runTUIForm(i18nText(locale, "keys.rename"), []tuiField{
		{label: i18nText(locale, "form.apiName"), kind: tuiInput},
		{label: i18nText(locale, "form.finish"), kind: tuiSelect, options: []string{i18nText(locale, "common.save"), i18nText(locale, "common.back")}, value: i18nText(locale, "common.save")},
	}, input, output)
	if err != nil {
		return err
	}
	if result.values[1] == i18nText(locale, "common.back") {
		return errUserAborted
	}
	if err := validateRequiredLocale(locale, i18nText(locale, "form.apiName"), result.values[0]); err != nil {
		return err
	}
	return request(http.MethodPut, "/api/keys/"+key.ID, map[string]string{"name": strings.TrimSpace(result.values[0])}, nil)
}

func (ui *UI) promptCombo(item *combo, edit bool) error {
	return ui.promptComboIO(item, edit, ui.In, ui.Out)
}

func (ui *UI) promptComboIO(item *combo, edit bool, input io.Reader, output io.Writer) error {
	models := comboModels(*item)
	options, err := ui.comboModelOptions()
	if err != nil {
		return err
	}
	result, err := ui.runTUIForm(ui.t("screen.createCombo"), []tuiField{
		{label: ui.t("screen.comboName"), kind: tuiInput, value: item.Name},
		{label: ui.t("screen.modelsList"), kind: tuiMultiSelect, options: options, selected: selectedToMarks(options, models)},
		{label: ui.t("form.finish"), kind: tuiSelect, options: []string{ui.t("common.save"), ui.t("common.back")}, value: ui.t("common.save")},
	}, input, output)
	if err != nil {
		return err
	}
	if result.values[2] == ui.t("common.back") {
		return errUserAborted
	}
	if err := validateComboValues(result.values[0], result.selected[1]); err != nil {
		return err
	}
	item.Name = result.values[0]
	item.Models = make([]any, len(result.selected[1]))
	for index, model := range result.selected[1] {
		item.Models[index] = model
	}
	return ui.saveCombo(item, edit, result.selected[1])
}

func i18nText(locale, key string) string {
	return i18n.T(locale, key)
}

func huhActionError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}
