package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"g9router/internal/providers"
)

var cursorMachineIDPattern = regexp.MustCompile(`^[a-f0-9]{32,}$`)

func (s *Server) cursorImportAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"provider": "cursor", "method": "import_token", "requiredFields": []map[string]string{{"name": "accessToken", "label": "Access Token", "type": "textarea"}, {"name": "machineId", "label": "Machine ID", "type": "text"}}})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		AccessToken string `json:"accessToken"`
		MachineID   string `json:"machineId"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.AccessToken) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Access token is required"})
		return
	}
	if strings.TrimSpace(input.MachineID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Machine ID is required"})
		return
	}
	token := strings.TrimSpace(input.AccessToken)
	machineID := strings.TrimSpace(input.MachineID)
	if len(token) < 50 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Invalid token format. Token appears too short."})
		return
	}
	if !cursorMachineIDPattern.MatchString(strings.ReplaceAll(strings.ToLower(machineID), "-", "")) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Invalid machine ID format. Expected UUID format."})
		return
	}
	data := map[string]any{"machineId": machineID, "authMethod": "imported", "provider": "Imported"}
	if claims := cursorTokenClaims(token); claims != nil {
		data["userId"] = claims["sub"]
	}
	if err := s.store.Upsert(providers.Provider{ID: "cursor", Name: "Cursor", BaseURL: "https://api2.cursor.sh", APIKey: token, APIType: "cursor", Enabled: true, TestStatus: "active", ProviderSpecificData: data}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "cursor", "machineId": machineID})
}

func cursorTokenClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}
