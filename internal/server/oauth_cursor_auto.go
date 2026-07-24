package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (s *Server) cursorAutoImportAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "error": err.Error()})
		return
	}
	candidates := cursorDatabasePaths(home)
	var dbPath string
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			dbPath = candidate
			break
		}
	}
	if dbPath == "" {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "error": "Cursor database not found", "checked": candidates})
		return
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "windowsManual": true, "dbPath": dbPath})
		return
	}
	defer database.Close()
	query := func(key string) string {
		var value string
		if database.QueryRowContext(r.Context(), "SELECT value FROM itemTable WHERE key=? LIMIT 1", key).Scan(&value) != nil {
			return ""
		}
		return normalizeCursorValue(value)
	}
	accessToken := query("cursorAuth/accessToken")
	if accessToken == "" {
		accessToken = query("cursorAuth/token")
	}
	machineID := query("storage.serviceMachineId")
	if machineID == "" {
		machineID = query("storage.machineId")
	}
	if machineID == "" {
		machineID = query("telemetry.machineId")
	}
	if accessToken == "" || machineID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "windowsManual": true, "dbPath": dbPath})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": true, "accessToken": accessToken, "machineId": machineID, "dbPath": dbPath})
}

func cursorDatabasePaths(home string) []string {
	if runtime.GOOS == "darwin" {
		return []string{filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb"), filepath.Join(home, "Library", "Application Support", "Cursor - Insiders", "User", "globalStorage", "state.vscdb")}
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		return []string{filepath.Join(appData, "Cursor", "User", "globalStorage", "state.vscdb"), filepath.Join(appData, "Cursor - Insiders", "User", "globalStorage", "state.vscdb"), filepath.Join(local, "Cursor", "User", "globalStorage", "state.vscdb"), filepath.Join(local, "Programs", "Cursor", "User", "globalStorage", "state.vscdb")}
	}
	return []string{filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb"), filepath.Join(home, ".config", "cursor", "User", "globalStorage", "state.vscdb")}
}

func normalizeCursorValue(value string) string {
	var parsed string
	if json.Unmarshal([]byte(value), &parsed) == nil {
		return parsed
	}
	return strings.TrimSpace(value)
}
