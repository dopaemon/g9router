package tui

import (
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
	accessible := os.Getenv("G9ROUTER_ACCESSIBLE") == "1"
	if file, ok := input.(*os.File); !ok || !IsTerminal(file) {
		accessible = true
	}
	return form.WithAccessible(accessible).WithInput(input).WithOutput(output).WithTheme(huh.ThemeCharm()).WithWidth(72).Run()
}

func (ui *UI) promptProvider(item *provider, edit bool) error {
	return ui.promptProviderIO(item, edit, ui.In, ui.Out)
}

func (ui *UI) promptProviderIO(item *provider, edit bool, input io.Reader, output io.Writer) error {
	apiType := item.APIType
	finish := "Save"
	if apiType == "" {
		apiType = "openai"
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Provider ID").Value(&item.ID).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Display name").Value(&item.Name),
		huh.NewInput().Title("Base URL").Value(&item.BaseURL).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("API key").EchoMode(huh.EchoModePassword).Value(&item.APIKey).Validate(huh.ValidateNotEmpty()),
		huh.NewSelect[string]().Title("API type").Options(
			huh.NewOption("OpenAI-compatible", "openai"),
			huh.NewOption("Anthropic", "anthropic"),
			huh.NewOption("Gemini", "gemini"),
		).Value(&apiType),
		huh.NewSelect[string]().Title("Finish").Options(
			huh.NewOption("Save", "Save"),
			huh.NewOption("Back", "Back"),
		).Value(&finish),
	))
	if err := ui.runHuhIO(form, input, output); err != nil {
		return err
	}
	if finish == "Back" {
		return huh.ErrUserAborted
	}
	item.APIType = apiType
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
		huh.NewInput().Title(i18n.T(locale, "form.apiName")).Value(&name).Validate(huh.ValidateNotEmpty()),
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
		huh.NewInput().Title(i18n.T(locale, "form.apiName")).Value(&name).Validate(huh.ValidateNotEmpty()),
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
	return request(http.MethodPut, "/api/keys/"+key.ID, map[string]string{"name": strings.TrimSpace(name)}, nil)
}

func (ui *UI) promptCombo(item *combo, edit bool) error {
	models := make([]string, 0, len(item.Models))
	for _, model := range item.Models {
		if value, ok := model.(string); ok {
			models = append(models, value)
		}
	}
	modelText := strings.Join(models, ", ")
	finish := "Save"
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Combo name").Value(&item.Name).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Models").Description("Comma-separated model IDs").Value(&modelText).Validate(huh.ValidateNotEmpty()),
		huh.NewSelect[string]().Title("Finish").Options(
			huh.NewOption("Save", "Save"),
			huh.NewOption("Back", "Back"),
		).Value(&finish),
	))
	if err := ui.runHuh(form); err != nil {
		return err
	}
	if finish == "Back" {
		return huh.ErrUserAborted
	}
	item.Models = nil
	for _, model := range strings.Split(modelText, ",") {
		if value := strings.TrimSpace(model); value != "" {
			item.Models = append(item.Models, value)
		}
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
