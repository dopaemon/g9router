package server

import (
	"net/http"
	"os"
	"strings"
	"time"
)

func (s *Server) shutdownAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if strings.EqualFold(os.Getenv("NODE_ENV"), "production") {
		writeJSON(w, http.StatusForbidden, map[string]any{"success": false, "message": "Not allowed in production"})
		return
	}
	secret := os.Getenv("SHUTDOWN_SECRET")
	authorization := r.Header.Get("Authorization")
	if secret == "" || authorization != "Bearer "+secret {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "Unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Shutting down..."})
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}
