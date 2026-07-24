package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
)

var audioVoiceAliases = map[string]string{
	"elevenlabs":   "el",
	"deepgram":     "dg",
	"inworld":      "iw",
	"edge-tts":     "edge-tts",
	"local-device": "local-device",
}

func (s *Server) audioVoicesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider := r.URL.Query().Get("provider")
	alias, ok := audioVoiceAliases[provider]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "provider must be one of: elevenlabs, deepgram, inworld, edge-tts, local-device", "type": "invalid_request_error"}})
		return
	}
	if provider == "deepgram" {
		s.deepgramVoicesAPI(w, r, alias)
		return
	}
	if provider != "edge-tts" && provider != "elevenlabs" {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]string{"message": "voice provider is not implemented", "type": "server_error"}})
		return
	}
	internal := r.Clone(r.Context())
	internal.URL.Path = "/api/media-providers/tts/voices"
	recorder := httptest.NewRecorder()
	s.ttsVoicesAPI(recorder, internal)
	if recorder.Code < 200 || recorder.Code >= 300 {
		writeJSON(w, recorder.Code, map[string]any{"error": map[string]string{"message": strings.TrimSpace(recorder.Body.String()), "type": "server_error"}})
		return
	}
	var payload struct {
		Voices []map[string]any `json:"voices"`
		ByLang map[string]struct {
			Voices []map[string]any `json:"voices"`
		} `json:"byLang"`
	}
	if json.Unmarshal(recorder.Body.Bytes(), &payload) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]string{"message": "invalid voice catalog", "type": "server_error"}})
		return
	}
	voices := payload.Voices
	if r.URL.Query().Get("lang") == "" {
		voices = nil
		for _, group := range payload.ByLang {
			voices = append(voices, group.Voices...)
		}
	}
	data := make([]map[string]any, 0, len(voices))
	for _, voice := range voices {
		id, _ := voice["id"].(string)
		name, _ := voice["name"].(string)
		lang, _ := voice["lang"].(string)
		gender, _ := voice["gender"].(string)
		data = append(data, map[string]any{"id": id, "name": name, "lang": lang, "gender": gender, "model": alias + "/" + id})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Server) deepgramVoicesAPI(w http.ResponseWriter, r *http.Request, alias string) {
	provider, ok := s.store.Find("deepgram")
	if !ok || provider.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "No Deepgram connection found", "type": "invalid_request_error"}})
		return
	}
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.deepgram.com/v1/models", nil)
	request.Header.Set("Authorization", "Token "+provider.APIKey)
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Deepgram API error"})
		return
	}
	var payload struct {
		TTS []struct {
			CanonicalName string   `json:"canonical_name"`
			Name          string   `json:"name"`
			Languages     []string `json:"languages"`
			Metadata      struct {
				Tags []string `json:"tags"`
			} `json:"metadata"`
		} `json:"tts"`
	}
	if json.Unmarshal(data, &payload) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid Deepgram voice catalog"})
		return
	}
	voices := []map[string]any{}
	langFilter := r.URL.Query().Get("lang")
	for _, item := range payload.TTS {
		id := item.CanonicalName
		if id == "" {
			id = item.Name
		}
		languages := item.Languages
		if len(languages) == 0 {
			languages = []string{"en"}
		}
		for _, lang := range languages {
			if langFilter != "" && lang != langFilter {
				continue
			}
			gender := ""
			for _, tag := range item.Metadata.Tags {
				if tag == "masculine" || tag == "feminine" {
					gender = tag
					break
				}
			}
			voices = append(voices, map[string]any{"id": id, "name": item.Name, "lang": lang, "gender": gender, "model": alias + "/" + id})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": voices})
}
