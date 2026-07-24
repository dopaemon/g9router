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
	"minimax":      "minimax",
	"minimax-cn":   "minimax-cn",
	"edge-tts":     "edge-tts",
	"local-device": "local-device",
	"gemini":       "gemini",
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
	if provider == "inworld" {
		s.inworldVoicesAPI(w, r, alias)
		return
	}
	if provider == "minimax" || provider == "minimax-cn" {
		s.minimaxVoicesAPI(w, r, alias)
		return
	}
	if provider != "edge-tts" && provider != "elevenlabs" && provider != "local-device" && provider != "gemini" {
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

func (s *Server) specializedVoicesAPI(w http.ResponseWriter, r *http.Request) {
	provider := ""
	switch {
	case strings.Contains(r.URL.Path, "/deepgram/"):
		provider = "deepgram"
	case strings.Contains(r.URL.Path, "/elevenlabs/"):
		provider = "elevenlabs"
	case strings.Contains(r.URL.Path, "/inworld/"):
		provider = "inworld"
	case strings.Contains(r.URL.Path, "/minimax/"):
		provider = r.URL.Query().Get("provider")
		if provider != "minimax-cn" {
			provider = "minimax"
		}
	}
	query := r.URL.Query()
	query.Set("provider", provider)
	clone := r.Clone(r.Context())
	clone.URL.RawQuery = query.Encode()
	s.audioVoicesAPI(w, clone)
}

func (s *Server) inworldVoicesAPI(w http.ResponseWriter, r *http.Request, alias string) {
	provider, ok := s.store.Find("inworld")
	if !ok || provider.APIKey == "" {
		writeJSON(w, 400, map[string]string{"error": "No Inworld connection found"})
		return
	}
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.inworld.ai/tts/v1/voices", nil)
	request.Header.Set("Authorization", "Basic "+provider.APIKey)
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, 502, map[string]string{"error": "Inworld API error"})
		return
	}
	var payload struct {
		Voices []struct {
			VoiceID     string   `json:"voiceId"`
			DisplayName string   `json:"displayName"`
			Gender      string   `json:"gender"`
			Languages   []string `json:"languages"`
		} `json:"voices"`
	}
	if json.Unmarshal(data, &payload) != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid Inworld voice catalog"})
		return
	}
	langFilter := r.URL.Query().Get("lang")
	voices := []map[string]any{}
	for _, item := range payload.Voices {
		langs := item.Languages
		if len(langs) == 0 {
			langs = []string{"en"}
		}
		for _, lang := range langs {
			if langFilter != "" && lang != langFilter {
				continue
			}
			voices = append(voices, map[string]any{"id": item.VoiceID, "name": item.DisplayName, "lang": lang, "gender": item.Gender, "model": alias + "/" + item.VoiceID})
		}
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": voices})
}

func (s *Server) minimaxVoicesAPI(w http.ResponseWriter, r *http.Request, alias string) {
	provider := r.URL.Query().Get("provider")
	if provider != "minimax-cn" {
		provider = "minimax"
	}
	connection, ok := s.store.Find(provider)
	if !ok || connection.APIKey == "" {
		writeJSON(w, 400, map[string]string{"error": "No " + provider + " connection found"})
		return
	}
	endpoint := "https://api.minimax.io/v1/get_voice"
	if provider == "minimax-cn" {
		endpoint = "https://api.minimaxi.com/v1/get_voice"
	}
	body, _ := json.Marshal(map[string]string{"voice_type": nonEmpty(r.URL.Query().Get("voice_type"), "all")})
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer "+connection.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, 502, map[string]string{"error": "MiniMax API error"})
		return
	}
	var payload map[string]any
	if json.Unmarshal(data, &payload) != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid MiniMax voice catalog"})
		return
	}
	langFilter := r.URL.Query().Get("lang")
	voices := []map[string]any{}
	groups := []string{"system_voice", "voice_cloning", "voice_generation", "music_generation"}
	for _, group := range groups {
		items, _ := payload[group].([]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			id := stringValue(item["voice_id"])
			if id == "" {
				id = stringValue(item["voiceId"])
			}
			if id == "" {
				continue
			}
			lang := "Custom"
			if group == "system_voice" && strings.Contains(id, "_") {
				lang = strings.SplitN(id, "_", 2)[0]
			}
			if langFilter != "" && lang != langFilter {
				continue
			}
			name := stringValue(item["voice_name"])
			if name == "" {
				name = stringValue(item["voiceName"])
			}
			if name == "" {
				name = id
			}
			voices = append(voices, map[string]any{"id": id, "name": name, "lang": lang, "category": group, "model": alias + "/" + id})
		}
	}
	writeJSON(w, 200, map[string]any{"object": "list", "data": voices})
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
