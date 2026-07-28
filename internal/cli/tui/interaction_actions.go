package tui

import (
	"net/http"
	"strings"
)

func (ui *UI) saveProvider(item *provider, edit bool) error {
	item.ID = strings.TrimSpace(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.BaseURL = strings.TrimSpace(item.BaseURL)
	item.APIKey = strings.TrimSpace(item.APIKey)
	if err := validateProviderValuesLocale(ui.Locale, item.ID, item.BaseURL, item.APIKey); err != nil {
		return err
	}
	item.Enabled = true
	method, path := http.MethodPost, "/api/providers"
	if edit {
		method, path = http.MethodPut, "/api/providers/"+item.ID
	}
	return ui.request(method, path, item, nil)
}

func (ui *UI) saveCombo(item *combo, edit bool, models []string) error {
	item.Name = strings.TrimSpace(item.Name)
	item.Models = make([]any, len(models))
	for index, model := range models {
		item.Models[index] = model
	}
	if err := validateComboValuesLocale(ui.Locale, item.Name, models); err != nil {
		return err
	}
	method, path := http.MethodPost, "/api/combos"
	if edit {
		method, path = http.MethodPut, "/api/combos/"+item.ID
	}
	return ui.request(method, path, item, nil)
}
