package server

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var googleTranslateToken struct {
	sync.Mutex
	fSID, bl string
	at       time.Time
}

var googleTranslateSID = regexp.MustCompile(`"FdrFJe":"(.*?)"`)
var googleTranslateBL = regexp.MustCompile(`"cfb2h":"(.*?)"`)
var googleTranslateIndex atomic.Uint64

func (s *Server) providerSpeech(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	switch providerID {
	case "elevenlabs":
		return s.elevenLabsSpeech(w, r, apiKey, input)
	case "gemini":
		return s.geminiSpeech(w, r, apiKey, input)
	case "google-tts":
		return s.googleTranslateSpeech(w, r, input)
	default:
		return false
	}
}

func (s *Server) googleTranslateSpeech(w http.ResponseWriter, r *http.Request, input map[string]any) bool {
	model, _ := input["model"].(string)
	lang := model
	if strings.Contains(lang, "/") {
		lang = strings.TrimPrefix(lang, "google-tts/")
	}
	if lang == "" || lang == "google-tts" {
		lang = "en"
	}
	text, _ := input["input"].(string)
	fSID, bl, ok := s.googleTranslateToken(r)
	if !ok {
		return false
	}
	requestID := googleTranslateIndex.Add(1)*100000 + uint64(time.Now().UnixNano()%9000) + 1000
	query := url.Values{"rpcids": {"jQ1olc"}, "f.sid": {fSID}, "bl": {bl}, "hl": {lang}, "soc-app": {"1"}, "soc-platform": {"1"}, "soc-device": {"1"}, "_reqid": {strconv.FormatUint(requestID, 10)}, "rt": {"c"}}
	clean := strings.NewReplacer("@", " ", "^", " ", "*", " ", "(", " ", ")", " ", `\`, " ", "/", " ", "-", " ", "_", " ", "+", " ", "=", " ", ">", " ", "<", " ", `"`, " ", "'", " ", "“", " ", "”", " ", "【", " ", "】", " ").Replace(text)
	clean = strings.ReplaceAll(clean, ", ", ". ")
	payload, _ := json.Marshal([]any{clean, lang, nil, "undefined", []int{0}})
	form := url.Values{}
	outerPayload, _ := json.Marshal([]any{[][]any{{"jQ1olc", string(payload), nil, "generic"}}})
	form.Set("f.req", string(outerPayload))
	endpoint := "https://translate.google.com/_/TranslateWebserverUi/data/batchexecute?" + query.Encode()
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Referer", "https://translate.google.com/")
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) < 4 {
		return false
	}
	var outer []any
	if json.Unmarshal([]byte(lines[3]), &outer) != nil || len(outer) == 0 {
		return false
	}
	row, ok := outer[0].([]any)
	if !ok || len(row) < 3 {
		return false
	}
	encoded, ok := row[2].(string)
	if !ok {
		return false
	}
	var parts []any
	if json.Unmarshal([]byte(encoded), &parts) != nil || len(parts) == 0 {
		return false
	}
	value, ok := parts[0].(string)
	if !ok || len(value) < 100 {
		return false
	}
	audio, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	writeSpeechAudio(w, input, audio, "mp3")
	return true
}

func (s *Server) googleTranslateToken(r *http.Request) (string, string, bool) {
	googleTranslateToken.Lock()
	if googleTranslateToken.fSID != "" && time.Since(googleTranslateToken.at) < 11*time.Minute {
		fSID, bl := googleTranslateToken.fSID, googleTranslateToken.bl
		googleTranslateToken.Unlock()
		return fSID, bl, true
	}
	googleTranslateToken.Unlock()
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://translate.google.com/", nil)
	if err != nil {
		return "", "", false
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	response, err := s.client.Do(request)
	if err != nil {
		return "", "", false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	fSIDMatch, blMatch := googleTranslateSID.FindSubmatch(data), googleTranslateBL.FindSubmatch(data)
	if response.StatusCode < 200 || response.StatusCode >= 300 || len(fSIDMatch) < 2 || len(blMatch) < 2 {
		return "", "", false
	}
	fSID, bl := string(fSIDMatch[1]), string(blMatch[1])
	googleTranslateToken.Lock()
	googleTranslateToken.fSID, googleTranslateToken.bl, googleTranslateToken.at = fSID, bl, time.Now()
	googleTranslateToken.Unlock()
	return fSID, bl, true
}

func (s *Server) elevenLabsSpeech(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	if apiKey == "" {
		return false
	}
	model, _ := input["model"].(string)
	modelID, voiceID := "eleven_flash_v2_5", model
	if strings.Contains(model, "/") {
		parts := strings.SplitN(model, "/", 2)
		modelID, voiceID = parts[0], parts[1]
	}
	if voiceID == "elevenlabs" || voiceID == "" {
		return false
	}
	text, _ := input["input"].(string)
	body, _ := json.Marshal(map[string]any{"text": text, "model_id": modelID, "voice_settings": map[string]float64{"stability": 0.5, "similarity_boost": 0.75}})
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.elevenlabs.io/v1/text-to-speech/"+url.PathEscape(voiceID), strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	request.Header.Set("xi-api-key", apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	audio, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || len(audio) < 1024 {
		return false
	}
	writeSpeechAudio(w, input, audio, "mp3")
	return true
}

func (s *Server) geminiSpeech(w http.ResponseWriter, r *http.Request, apiKey string, input map[string]any) bool {
	if apiKey == "" {
		return false
	}
	model, _ := input["model"].(string)
	modelID, voiceID := "gemini-3.1-flash-tts-preview", "Kore"
	known := []string{"gemini-3.1-flash-tts-preview", "gemini-2.5-flash-preview-tts", "gemini-2.5-pro-preview-tts"}
	for _, candidate := range known {
		if model == candidate {
			modelID = candidate
		} else if strings.HasPrefix(model, candidate+"/") {
			modelID, voiceID = candidate, strings.TrimPrefix(model, candidate+"/")
		}
	}
	if model != "gemini" && model != "" && modelID == "gemini-3.1-flash-tts-preview" && !strings.HasPrefix(model, "gemini-") {
		voiceID = model
	}
	text, _ := input["input"].(string)
	body, _ := json.Marshal(map[string]any{"contents": []map[string]any{{"parts": []map[string]string{{"text": "Say: " + text}}}}, "generationConfig": map[string]any{"responseModalities": []string{"AUDIO"}, "speechConfig": map[string]any{"voiceConfig": map[string]any{"prebuiltVoiceConfig": map[string]string{"voiceName": voiceID}}}}})
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/" + url.PathEscape(modelID) + ":generateContent?key=" + url.QueryEscape(apiKey)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						Data string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &payload) != nil || len(payload.Candidates) == 0 || len(payload.Candidates[0].Content.Parts) == 0 {
		return false
	}
	pcm, err := base64.StdEncoding.DecodeString(payload.Candidates[0].Content.Parts[0].InlineData.Data)
	if err != nil || len(pcm) == 0 {
		return false
	}
	writeSpeechAudio(w, input, pcmToWAV(pcm, 24000, 1, 16), "wav")
	return true
}

func writeSpeechAudio(w http.ResponseWriter, input map[string]any, audio []byte, format string) {
	if input["response_format"] == "json" {
		writeJSON(w, http.StatusOK, map[string]string{"audio": base64.StdEncoding.EncodeToString(audio), "format": format})
		return
	}
	w.Header().Set("Content-Type", "audio/"+format)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

func pcmToWAV(pcm []byte, sampleRate, channels, bits int) []byte {
	dataSize := len(pcm)
	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+dataSize))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:], uint32(sampleRate))
	byteRate := sampleRate * channels * bits / 8
	binary.LittleEndian.PutUint32(header[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(header[32:], uint16(channels*bits/8))
	binary.LittleEndian.PutUint16(header[34:], uint16(bits))
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(dataSize))
	return append(header, pcm...)
}
