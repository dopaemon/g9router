package tui

import (
	"strings"
)

type tunnelStatus struct {
	Tunnel struct {
		Enabled   bool   `json:"enabled"`
		PublicURL string `json:"publicUrl"`
	} `json:"tunnel"`
	Tailscale struct {
		Enabled   bool   `json:"enabled"`
		TunnelURL string `json:"tunnelUrl"`
	} `json:"tailscale"`
}

type tailscaleStatus struct {
	Installed bool `json:"installed"`
}

func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

func tailscaleState(url string, installed bool) string {
	if !installed {
		return "not installed"
	}
	return strings.TrimSpace(url)
}

func apiEndpoint(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" || value == "not installed" || strings.HasSuffix(value, "/v1") {
		return value
	}
	return value + "/v1"
}
