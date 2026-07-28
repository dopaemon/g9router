package tui

import (
	"errors"
	"fmt"
	"strings"

	"g9router/internal/i18n"
)

func validateRequiredLocale(locale, label, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf(i18n.T(locale, "form.required"), label)
	}
	return nil
}

func validateProviderValues(id, baseURL, apiKey string) error {
	return validateProviderValuesLocale(i18n.English, id, baseURL, apiKey)
}

func validateProviderValuesLocale(locale, id, baseURL, apiKey string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
		return errors.New(i18n.T(locale, "form.providerValuesRequired"))
	}
	return nil
}

func validateComboValues(name string, models []string) error {
	return validateComboValuesLocale(i18n.English, name, models)
}

func validateComboValuesLocale(locale, name string, models []string) error {
	if strings.TrimSpace(name) == "" || len(models) == 0 {
		return errors.New(i18n.T(locale, "form.comboValuesRequired"))
	}
	return nil
}
