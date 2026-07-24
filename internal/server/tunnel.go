package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func (s *Server) tunnelStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	publicURL, pid, running := s.tunnelManager.Status()
	settings := s.settings.Get()
	settingsEnabled, _ := settings["tunnelEnabled"].(bool)
	if configured, ok := settings["tunnelUrl"].(string); ok && publicURL == "" {
		publicURL = configured
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tunnel":    map[string]any{"enabled": running, "settingsEnabled": settingsEnabled, "publicUrl": publicURL, "pid": pid},
		"tailscale": map[string]any{"enabled": settings["tailscaleEnabled"] == true, "tunnelUrl": settings["tailscaleUrl"]},
		"download":  map[string]any{"active": false},
	})
}

func (s *Server) tunnelEnableAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	publicURL, pid, err := s.tunnelManager.Start(context.Background(), "20128")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if err := s.settings.Update(map[string]any{"tunnelEnabled": true, "tunnelUrl": publicURL}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": true, "tunnelUrl": publicURL, "publicUrl": publicURL, "pid": pid})
}

func (s *Server) tunnelDisableAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	running := s.tunnelManager.Stop()
	if err := s.settings.Update(map[string]any{"tunnelEnabled": false, "tunnelUrl": ""}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": false, "stopped": running})
}

func tailscaleInstalled() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

func tailscalePlatform() string { return runtime.GOOS }

func (s *Server) tailscaleCheckAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	status := tailscaleProbe()
	writeJSON(w, http.StatusOK, map[string]any{
		"installed": tailscaleInstalled(), "platform": tailscalePlatform(),
		"loggedIn": status.loggedIn, "running": status.running, "tunnelUrl": status.url,
	})
}

func (s *Server) tailscaleEnableAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !tailscaleInstalled() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "tailscale is not installed"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "tailscale", "funnel", "--bg", "20128").CombinedOutput(); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": strings.TrimSpace(string(output))})
		return
	}
	status := tailscaleProbe()
	if err := s.settings.Update(map[string]any{"tailscaleEnabled": true, "tailscaleUrl": status.url}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": status.running, "tunnelUrl": status.url})
}

func (s *Server) tailscaleDisableAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if tailscaleInstalled() {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if output, err := exec.CommandContext(ctx, "tailscale", "funnel", "off").CombinedOutput(); err != nil && strings.TrimSpace(string(output)) != "" {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": strings.TrimSpace(string(output))})
			return
		}
	}
	if err := s.settings.Update(map[string]any{"tailscaleEnabled": false, "tailscaleUrl": ""}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": false})
}

func (s *Server) tailscaleInstallAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "automatic Tailscale installation is unavailable; install it from tailscale.com"})
}

type tailscaleStatus struct {
	loggedIn, running bool
	url               string
}

func tailscaleProbe() tailscaleStatus {
	result := tailscaleStatus{}
	if !tailscaleInstalled() {
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output(); err == nil {
		var payload struct {
			BackendState string `json:"BackendState"`
			Self         struct {
				Online  bool   `json:"Online"`
				DNSName string `json:"DNSName"`
			} `json:"Self"`
		}
		if json.Unmarshal(output, &payload) == nil {
			result.loggedIn = payload.BackendState == "Running" && payload.Self.Online
			result.url = "https://" + strings.TrimSuffix(payload.Self.DNSName, ".")
		}
	}
	if output, err := exec.CommandContext(ctx, "tailscale", "funnel", "status", "--json").Output(); err == nil {
		var payload struct {
			AllowFunnel map[string]any `json:"AllowFunnel"`
		}
		result.running = json.Unmarshal(output, &payload) == nil && len(payload.AllowFunnel) > 0
	}
	return result
}
