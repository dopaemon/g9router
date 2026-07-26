package server

import (
	"encoding/json"
	"net/http"
)

var supportedLocales = map[string]bool{
	"en": true, "vi": true,
}

func (s *Server) localeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Locale string `json:"locale"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || !supportedLocales[input.Locale] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid locale"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "locale", Value: input.Locale, Path: "/", MaxAge: 365 * 24 * 60 * 60})
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "locale": input.Locale})
}
