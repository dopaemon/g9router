package server

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
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
