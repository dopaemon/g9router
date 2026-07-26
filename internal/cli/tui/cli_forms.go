package tui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/huh"
)

func (ui *UI) runHuh(form *huh.Form) error {
	return form.WithInput(ui.In).WithOutput(ui.Out).WithTheme(huh.ThemeCharm()).WithWidth(72).Run()
}

func (ui *UI) promptProvider(item *provider, edit bool) error {
	apiType := item.APIType
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
	)).WithAccessible(false)
	if err := ui.runHuh(form); err != nil {
		return err
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
	var name string
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Key name").Value(&name).Validate(huh.ValidateNotEmpty()),
	)).WithAccessible(false)
	if err := ui.runHuh(form); err != nil {
		return apiKey{}, err
	}
	var created apiKey
	err := ui.request(http.MethodPost, "/api/keys", map[string]string{"name": strings.TrimSpace(name)}, &created)
	return created, err
}

func (ui *UI) promptCombo(item *combo, edit bool) error {
	models := make([]string, 0, len(item.Models))
	for _, model := range item.Models {
		if value, ok := model.(string); ok {
			models = append(models, value)
		}
	}
	modelText := strings.Join(models, ", ")
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Combo name").Value(&item.Name).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Models").Description("Comma-separated model IDs").Value(&modelText).Validate(huh.ValidateNotEmpty()),
	)).WithAccessible(false)
	if err := ui.runHuh(form); err != nil {
		return err
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
