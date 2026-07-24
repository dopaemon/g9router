package server

import (
	"encoding/json"
	"net/http"
)

var supportedLocales = map[string]bool{
	"en": true, "vi": true, "zh-CN": true, "zh-TW": true, "ja": true, "pt-BR": true, "pt-PT": true,
	"ko": true, "es": true, "de": true, "fr": true, "he": true, "ar": true, "ru": true, "pl": true,
	"cs": true, "nl": true, "tr": true, "uk": true, "tl": true, "id": true, "th": true, "km": true,
	"hi": true, "bn": true, "ur": true, "ro": true, "sv": true, "it": true, "el": true, "hu": true,
	"fi": true, "da": true, "no": true, "fa": true,
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
