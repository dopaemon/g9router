package tui

import (
	"io"
	"os"

	"g9router/internal/i18n"
)

func ResetPasswordForm(input io.Reader, output io.Writer) (string, error) {
	ui := &UI{In: input, Out: output, Locale: i18n.Normalize(os.Getenv("G9ROUTER_LOCALE"))}
	result, err := ui.runTUIForm(ui.t("settings.setPassword"), []tuiField{
		{label: ui.t("form.newPassword"), kind: tuiInput, password: true},
		{label: ui.t("form.confirmPassword"), kind: tuiInput, password: true},
	}, input, output)
	if err != nil {
		return "", err
	}
	if result.values[0] != result.values[1] {
		return "", os.ErrInvalid
	}
	return result.values[0], nil
}
