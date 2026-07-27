package tui

import (
	"errors"
	"fmt"
	"strings"
)

func validateRequired(label, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", label)
	}
	return nil
}

func validateProviderValues(id, baseURL, apiKey string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
		return errors.New("provider ID, base URL, and API key are required")
	}
	return nil
}

func validateComboValues(name string, models []string) error {
	if strings.TrimSpace(name) == "" || len(models) == 0 {
		return errors.New("combo name and at least one model are required")
	}
	return nil
}
