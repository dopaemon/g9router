package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) antigravityMITMAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := s.mitmManager.Status()
		writeJSON(w, http.StatusOK, map[string]any{"running": status.Running, "pid": status.PID, "certExists": status.CertExists, "certTrusted": status.CertTrusted, "dnsStatus": s.mitmManager.DNSStatus(), "isWin": false, "isAdmin": false, "listenAddress": status.ListenAddress, "mitmRouterBaseUrl": status.RouterBaseURL})
	case http.MethodPost:
		var input struct {
			APIKey        string `json:"apiKey"`
			RouterBaseURL string `json:"mitmRouterBaseUrl"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.APIKey) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing apiKey"})
			return
		}
		base := strings.TrimSpace(input.RouterBaseURL)
		if base == "" {
			base = "http://localhost:20128"
		}
		status, err := s.mitmManager.Start(base, input.APIKey)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "running": status.Running, "pid": status.PID, "listenAddress": status.ListenAddress})
	case http.MethodDelete:
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := s.mitmManager.Stop(ctx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "running": false})
	case http.MethodPatch:
		var input struct {
			Tool         string `json:"tool"`
			Action       string `json:"action"`
			SudoPassword string `json:"sudoPassword"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Tool) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool and action required"})
			return
		}
		if input.Action == "trust-cert" {
			if err := s.mitmManager.TrustCertificate(input.SudoPassword); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
				return
			}
			status := s.mitmManager.Status()
			status.CertTrusted = true
			writeJSON(w, http.StatusOK, map[string]any{"success": true, "certTrusted": status.CertTrusted})
			return
		}
		if input.Action != "enable" && input.Action != "disable" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be enable or disable"})
			return
		}
		if err := s.mitmManager.SetDNS(input.Tool, input.SudoPassword, input.Action == "enable"); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "dnsStatus": s.mitmManager.DNSStatus()})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
