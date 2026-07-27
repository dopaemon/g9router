package tui

import (
	"fmt"
	"strings"
)

func (ui *UI) errorView(title string, err error) string {
	message := errorStyle.Render(ui.t("common.error") + ": " + ui.errorSummary(err))
	actions := mutedStyle.Render(ui.t("common.retryBack"))
	return ui.outerStyle().Render(cardTitleStyle.Render(title) + "\n\n" + message + "\n\n" + actions)
}

func (ui *UI) errorSummary(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return ui.t("error.auth")
	case strings.Contains(lower, "403") || strings.Contains(lower, "forbidden"):
		return ui.t("error.permission")
	case strings.Contains(lower, "404") || strings.Contains(lower, "not found"):
		return ui.t("error.notFound")
	case strings.Contains(lower, "500") || strings.Contains(lower, "502") || strings.Contains(lower, "503"):
		return ui.t("error.server")
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "connection"):
		return ui.t("error.network")
	default:
		return fmt.Sprint(err)
	}
}
