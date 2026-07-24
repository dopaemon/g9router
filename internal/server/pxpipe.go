package server

import (
	"net/http"
	"strconv"
)

func (s *Server) pxpipeStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	settings := s.settings.Get()
	status := s.pxpipeManager.Status()
	status["enabled"] = settings["pxpipeEnabled"] == true
	status["autoInstall"] = settings["pxpipeAutoInstall"] == true
	status["minChars"] = settings["pxpipeMinChars"]
	status["timeoutMs"] = settings["pxpipeTimeoutMs"]
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) pxpipeStatsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	limit := pxpipeLimit(r)
	events := s.pxpipeManager.Events(limit)
	writeJSON(w, http.StatusOK, map[string]any{"total": len(events), "events": events})
}

func (s *Server) pxpipeLogsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installLog": []string{}, "events": s.pxpipeManager.Events(pxpipeLimit(r))})
}

func (s *Server) pxpipeHealthAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	status := s.pxpipeManager.Status()
	writeJSON(w, http.StatusOK, map[string]any{"healthy": status["available"] == true, "checks": []any{map[string]any{"name": "module", "ok": false, "detail": "PXPIPE native module is not installed"}}})
}

func (s *Server) pxpipeInstallAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	result, err := s.pxpipeManager.Install(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "info": result, "health": map[string]any{"healthy": false, "checks": []any{map[string]any{"name": "module", "ok": false, "detail": "PXPIPE native module requires runtime integration"}}}})
}

func (s *Server) pxpipeStartAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	settings := s.settings.Get()
	if status := s.pxpipeManager.Status(); status["installed"] != true && settings["pxpipeAutoInstall"] != true {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "PXPIPE is not installed", "code": "NOT_INSTALLED"})
		return
	}
	writeJSON(w, http.StatusOK, s.pxpipeManager.Start())
}

func (s *Server) pxpipeStopAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": s.pxpipeManager.Stop(), "status": s.pxpipeManager.Status()})
}

func (s *Server) pxpipeRestartAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.pxpipeManager.Stop()
	writeJSON(w, http.StatusOK, s.pxpipeManager.Start())
}

func pxpipeLimit(r *http.Request) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}
