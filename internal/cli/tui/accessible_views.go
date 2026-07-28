package tui

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
)

func (ui *UI) accessibleEndpoint(reader *bufio.Reader) error {
	for {
		var status tunnelStatus
		if err := ui.request(http.MethodGet, "/api/tunnel/status", nil, &status); err != nil {
			return err
		}
		var tailscale tailscaleStatus
		if err := ui.request(http.MethodGet, "/api/tunnel/tailscale-check", nil, &tailscale); err != nil {
			return err
		}
		var payload struct {
			Keys []apiKey `json:"keys"`
		}
		if err := ui.request(http.MethodGet, "/api/keys", nil, &payload); err != nil {
			return err
		}
		fmt.Fprintln(ui.Out, "\n"+ui.t("endpoint.title"))
		fmt.Fprintln(ui.Out, endpointLine(ui, ui.t("endpoint.local"), apiEndpoint(ui.BaseURL)))
		fmt.Fprintln(ui.Out, endpointLine(ui, ui.t("endpoint.tunnel"), statusTextLocale(status.Tunnel.Enabled, ui.Locale)))
		fmt.Fprintln(ui.Out, endpointLine(ui, ui.t("endpoint.tailscale"), statusTextLocale(tailscale.Installed && status.Tailscale.Enabled, ui.Locale)))
		for index, key := range payload.Keys {
			fmt.Fprintln(ui.Out, formatLiveKey(index+1, key, ui.Locale))
		}
		choice, err := ui.readChoice(reader, ui.t("common.controls"), []string{ui.t("common.refresh"), ui.t("common.back")})
		if err != nil || strings.Contains(choice, ui.t("common.back")) {
			return err
		}
	}
}

func (ui *UI) accessibleStatistics(reader *bufio.Reader) error {
	var stats statisticsPayload
	if err := ui.request(http.MethodGet, "/api/usage/stats?period=today", nil, &stats); err != nil {
		return err
	}
	fmt.Fprintln(ui.Out, "\n"+ui.t("menu.statistics"))
	fmt.Fprintf(ui.Out, "%s: %d\n%s: %d\n%s: %d\n%s: %.4f\n", ui.t("stats.requests"), stats.TotalRequests, ui.t("stats.promptTokens"), stats.TotalPromptTokens, ui.t("stats.completionTokens"), stats.TotalCompletionTokens, ui.t("stats.estimatedCost"), stats.TotalCost)
	_, err := ui.readChoice(reader, ui.t("common.controls"), []string{ui.t("common.back")})
	return err
}

func (ui *UI) accessibleLogs(reader *bufio.Reader) error {
	var logs []apiLogEntry
	if err := ui.request(http.MethodGet, "/api/usage/logs", nil, &logs); err != nil {
		return err
	}
	fmt.Fprintln(ui.Out, "\n"+ui.t("menu.logs"))
	start := max(0, len(logs)-5)
	for index := start; index < len(logs); index++ {
		entry := logs[index]
		fmt.Fprintln(ui.Out, redactLogText(entry.Timestamp+" "+entry.Provider+"/"+entry.Model+" "+entry.Status))
	}
	_, err := ui.readChoice(reader, ui.t("common.controls"), []string{ui.t("common.back")})
	return err
}
