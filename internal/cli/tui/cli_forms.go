package tui

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"g9router/internal/i18n"
	"github.com/charmbracelet/huh"
)

func (ui *UI) runHuh(form *huh.Form) error {
	return ui.runHuhIO(form, ui.In, ui.Out)
}

func (ui *UI) runHuhIO(form *huh.Form, input io.Reader, output io.Writer) error {
	EnableColors(output)
	return form.WithAccessible(accessibleMode(input)).WithInput(input).WithOutput(output).WithTheme(huh.ThemeCharm()).WithWidth(ui.huhWidth()).Run()
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
	apiType := item.APIType
	finish := "Save"
	locale := ui.Locale
	if apiType == "" {
		apiType = "openai"
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(i18n.T(locale, "form.providerID")).Value(&item.ID).Validate(func(value string) error { return validateRequired(i18n.T(locale, "form.providerID"), value) }),
		huh.NewInput().Title(i18n.T(locale, "form.displayName")).Value(&item.Name),
		huh.NewInput().Title(i18n.T(locale, "form.baseURL")).Value(&item.BaseURL).Validate(func(value string) error { return validateRequired(i18n.T(locale, "form.baseURL"), value) }),
		huh.NewInput().Title(i18n.T(locale, "form.apiKey")).EchoMode(huh.EchoModePassword).Value(&item.APIKey).Validate(func(value string) error { return validateRequired(i18n.T(locale, "form.apiKey"), value) }),
		huh.NewSelect[string]().Title(i18n.T(locale, "form.apiType")).Options(
			huh.NewOption(i18n.T(locale, "form.openAICompatible"), "openai"),
			huh.NewOption(i18n.T(locale, "form.anthropic"), "anthropic"),
			huh.NewOption(i18n.T(locale, "form.gemini"), "gemini"),
		).Value(&apiType),
		huh.NewSelect[string]().Title(i18n.T(locale, "form.finish")).Options(
			huh.NewOption(i18n.T(locale, "common.save"), "Save"),
			huh.NewOption(i18n.T(locale, "common.back"), "Back"),
		).Value(&finish),
	))
	if err := ui.runHuhIO(form, input, output); err != nil {
		return err
	}
	if finish == "Back" {
		return huh.ErrUserAborted
	}
	item.APIType = apiType
	if err := validateProviderValues(item.ID, item.BaseURL, item.APIKey); err != nil {
		return err
	}
	item.Enabled = true
	method := http.MethodPost
	path := "/api/providers"
	if edit {
		method = http.MethodPut
		path = "/api/providers/" + item.ID
	}
	return ui.request(method, path, item, nil)
}

func (ui *UI) promptAPIKey() (apiKey, error) {
	return promptAPIKeyIO(ui.In, ui.Out, ui.runHuh, ui.request, ui.Locale)
}

func promptAPIKeyIO(input io.Reader, output io.Writer, run func(*huh.Form) error, request func(string, string, any, any) error, locale string) (apiKey, error) {
	var name string
	finish := "Save"
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(i18n.T(locale, "form.apiName")).Value(&name).Validate(func(value string) error { return validateRequired(i18n.T(locale, "form.apiName"), value) }),
		huh.NewSelect[string]().Title(i18n.T(locale, "form.finish")).Options(
			huh.NewOption(i18n.T(locale, "common.save"), "Save"),
			huh.NewOption(i18n.T(locale, "common.back"), "Back"),
		).Value(&finish),
	))
	if err := run(form); err != nil {
		return apiKey{}, err
	}
	if finish == "Back" {
		return apiKey{}, huh.ErrUserAborted
	}
	if err := validateRequired(i18n.T(locale, "form.apiName"), name); err != nil {
		return apiKey{}, err
	}
	var created apiKey
	err := request(http.MethodPost, "/api/keys", map[string]string{"name": strings.TrimSpace(name)}, &created)
	return created, err
}

func (ui *UI) promptAPIKeyRename(key *apiKey) error {
	return ui.promptAPIKeyRenameIO(key, ui.In, ui.Out, ui.runHuh, ui.request, ui.Locale)
}

func (ui *UI) promptAPIKeyRenameIO(key *apiKey, input io.Reader, output io.Writer, run func(*huh.Form) error, request func(string, string, any, any) error, locale string) error {
	name := key.Name
	finish := "Save"
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(i18n.T(locale, "form.apiName")).Value(&name).Validate(func(value string) error { return validateRequired(i18n.T(locale, "form.apiName"), value) }),
		huh.NewSelect[string]().Title(i18n.T(locale, "form.finish")).Options(
			huh.NewOption(i18n.T(locale, "common.save"), "Save"),
			huh.NewOption(i18n.T(locale, "common.back"), "Back"),
		).Value(&finish),
	))
	if err := run(form); err != nil {
		return err
	}
	if finish == "Back" {
		return huh.ErrUserAborted
	}
	if err := validateRequired(i18n.T(locale, "form.apiName"), name); err != nil {
		return err
	}
	return request(http.MethodPut, "/api/keys/"+key.ID, map[string]string{"name": strings.TrimSpace(name)}, nil)
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
	finish := "Save"
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(i18n.T(ui.Locale, "screen.comboName")).Value(&item.Name).Validate(func(value string) error { return validateRequired(i18n.T(ui.Locale, "screen.comboName"), value) }),
		huh.NewMultiSelect[string]().Title(i18n.T(ui.Locale, "screen.modelsList")).Options(options...).Value(&models).Validate(func(value []string) error {
			if len(value) == 0 {
				return errors.New(i18n.T(ui.Locale, "form.selectModels"))
			}
			return nil
		}),
		huh.NewSelect[string]().Title(i18n.T(ui.Locale, "form.finish")).Options(
			huh.NewOption(i18n.T(ui.Locale, "common.save"), "Save"),
			huh.NewOption(i18n.T(ui.Locale, "common.back"), "Back"),
		).Value(&finish),
	))
	if err := ui.runHuhIO(form, input, output); err != nil {
		return err
	}
	if finish == "Back" {
		return huh.ErrUserAborted
	}
	item.Models = make([]any, len(models))
	for index, model := range models {
		item.Models[index] = model
	}
	if err := validateComboValues(item.Name, models); err != nil {
		return err
	}
	method := http.MethodPost
	path := "/api/combos"
	if edit {
		method = http.MethodPut
		path = "/api/combos/" + item.ID
	}
	return ui.request(method, path, item, nil)
}

func huhActionError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}
