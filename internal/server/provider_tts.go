package server

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
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
	case "inworld", "cartesia", "playht", "coqui", "tortoise":
		return s.genericTTSSpeech(w, r, providerID, apiKey, input)
	case "deepgram", "nvidia", "huggingface":
		return s.binaryTTSSpeech(w, r, providerID, apiKey, input)
	case "minimax", "minimax-cn":
		return s.minimaxSpeech(w, r, providerID, apiKey, input)
	case "openai", "openrouter":
		return s.openAICompatibleSpeech(w, r, providerID, apiKey, input)
	default:
		return false
	}
}

func (s *Server) openAICompatibleSpeech(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	if apiKey == "" {
		return false
	}
	model := stringValue(input["model"])
	voice := "alloy"
	if index := strings.LastIndex(model, "/"); index >= 0 {
		voice = model[index+1:]
		model = model[:index]
	} else if model != "" {
		voice = model
	}
	if providerID == "openai" {
		model = nonEmpty(model, "tts-1")
		body, _ := json.Marshal(map[string]any{"model": model, "voice": voice, "input": stringValue(input["input"])})
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.openai.com/v1/audio/speech", strings.NewReader(string(body)))
		if err != nil {
			return false
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+apiKey)
		response, err := s.client.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		audio, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
		if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || len(audio) == 0 {
			return false
		}
		writeSpeechAudio(w, input, audio, "mp3")
		return true
	}
	model = nonEmpty(model, "openai/gpt-4o-mini-tts")
	body, _ := json.Marshal(map[string]any{"model": model, "modalities": []string{"text", "audio"}, "audio": map[string]string{"voice": voice, "format": "wav"}, "stream": true, "messages": []map[string]string{{"role": "user", "content": stringValue(input["input"])}}})
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("HTTP-Referer", "https://endpoint-proxy.local")
	request.Header.Set("X-Title", "Endpoint Proxy")
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var encoded strings.Builder
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 32<<20))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Audio struct {
						Data string `json:"data"`
					} `json:"audio"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil && len(event.Choices) > 0 {
			encoded.WriteString(event.Choices[0].Delta.Audio.Data)
		}
	}
	if encoded.Len() == 0 {
		return false
	}
	audio, err := base64.StdEncoding.DecodeString(encoded.String())
	if err != nil {
		return false
	}
	writeSpeechAudio(w, input, audio, "wav")
	return true
}

func (s *Server) minimaxSpeech(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	text := stringValue(input["input"])
	model := nonEmpty(stringValue(input["model"]), "speech-2.8-hd")
	voice := nonEmpty(stringValue(input["voice"]), "English_expressive_narrator")
	endpoint := "https://api.minimax.io/v1/t2a_v2"
	if providerID == "minimax-cn" {
		endpoint = "https://api.minimaxi.com/v1/t2a_v2"
	}
	body := map[string]any{"model": model, "text": text, "stream": false, "language_boost": "auto", "output_format": "hex", "voice_setting": map[string]any{"voice_id": voice, "speed": 1, "vol": 1, "pitch": 0}, "audio_setting": map[string]any{"sample_rate": 32000, "bitrate": 128000, "format": "mp3", "channel": 1}}
	encoded, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	var payload struct {
		Data struct {
			Audio string `json:"audio"`
		} `json:"data"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if json.Unmarshal(data, &payload) != nil || response.StatusCode < 200 || response.StatusCode >= 300 || payload.BaseResp.StatusCode != 0 || payload.Data.Audio == "" {
		return false
	}
	audio, err := hex.DecodeString(payload.Data.Audio)
	if err != nil || len(audio) == 0 {
		return false
	}
	writeSpeechAudio(w, input, audio, "mp3")
	return true
}

func (s *Server) binaryTTSSpeech(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	text, _ := input["input"].(string)
	model, _ := input["model"].(string)
	if strings.Contains(model, "/") {
		model = strings.SplitN(model, "/", 2)[1]
	}
	var endpoint string
	var request *http.Request
	var err error
	switch providerID {
	case "deepgram":
		endpoint = "https://api.deepgram.com/v1/speak?model=" + url.QueryEscape(nonEmpty(model, "aura-asteria-en"))
		request, err = http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, strings.NewReader(`{"text":`+strconv.Quote(text)+`}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Token "+apiKey)
	case "nvidia":
		voice, _ := input["voice"].(string)
		body, _ := json.Marshal(map[string]any{"input": map[string]string{"text": text}, "voice": nonEmpty(voice, "default"), "model": model})
		request, err = http.NewRequestWithContext(r.Context(), http.MethodPost, "https://integrate.api.nvidia.com/v1/audio/speech", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+apiKey)
	case "huggingface":
		if model == "" || strings.Contains(model, "..") || strings.Contains(model, "//") {
			return false
		}
		request, err = http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api-inference.huggingface.co/models/"+strings.TrimPrefix(model, "/"), strings.NewReader(`{"inputs":`+strconv.Quote(text)+`}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if err != nil {
		return false
	}
	response, err := s.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	audio, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || len(audio) == 0 {
		return false
	}
	format := "mp3"
	if providerID == "nvidia" || providerID == "huggingface" {
		format = "wav"
	}
	writeSpeechAudio(w, input, audio, format)
	return true
}

func (s *Server) genericTTSSpeech(w http.ResponseWriter, r *http.Request, providerID, apiKey string, input map[string]any) bool {
	text, _ := input["input"].(string)
	model, _ := input["model"].(string)
	voice, _ := input["voice"].(string)
	if strings.Contains(model, "/") {
		parts := strings.SplitN(model, "/", 2)
		model, voice = parts[0], parts[1]
	}
	if voice == "" {
		voice = "Alex"
	}
	var endpoint string
	var body map[string]any
	playHTUserID := ""
	request := func(method, endpoint string, body map[string]any) (*http.Request, error) {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return http.NewRequestWithContext(r.Context(), method, endpoint, strings.NewReader(string(encoded)))
	}
	switch providerID {
	case "inworld":
		endpoint = "https://api.inworld.ai/tts/v1/voice"
		body = map[string]any{"text": text, "voiceId": voice, "modelId": nonEmpty(model, "inworld-tts-1.5-mini"), "audioConfig": map[string]any{"audioEncoding": "MP3"}}
	case "cartesia":
		endpoint = "https://api.cartesia.ai/tts/bytes"
		body = map[string]any{"model_id": nonEmpty(model, "sonic-2"), "transcript": text, "voice": map[string]any{"mode": "id", "id": voice}, "output_format": map[string]any{"container": "mp3", "bit_rate": 128000, "sample_rate": 44100}}
	case "playht":
		endpoint = "https://api.play.ht/api/v2/tts/stream"
		parts := strings.SplitN(apiKey, ":", 2)
		if len(parts) == 2 {
			playHTUserID = parts[0]
			apiKey = parts[1]
		}
		body = map[string]any{"text": text, "voice": voice, "voice_engine": nonEmpty(model, "PlayDialog"), "output_format": "mp3", "speed": 1}
	case "coqui":
		endpoint = "http://localhost:5002/api/tts"
		body = map[string]any{"text": text, "speaker_id": voice}
	case "tortoise":
		endpoint = "http://localhost:5000/api/tts"
		body = map[string]any{"text": text, "voice": nonEmpty(voice, "random")}
	}
	requestObject, err := request(http.MethodPost, endpoint, body)
	if err != nil {
		return false
	}
	requestObject.Header.Set("Content-Type", "application/json")
	if providerID == "inworld" {
		requestObject.Header.Set("Authorization", "Basic "+apiKey)
	} else if providerID == "cartesia" {
		requestObject.Header.Set("X-API-Key", apiKey)
		requestObject.Header.Set("Cartesia-Version", "2024-06-10")
	} else if providerID == "playht" {
		requestObject.Header.Set("X-USER-ID", playHTUserID)
		requestObject.Header.Set("Authorization", "Bearer "+apiKey)
		requestObject.Header.Set("Accept", "audio/mpeg")
	}
	response, err := s.client.Do(requestObject)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	audio, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || len(audio) == 0 {
		return false
	}
	if providerID == "inworld" {
		var payload struct {
			AudioContent string `json:"audioContent"`
		}
		if json.Unmarshal(audio, &payload) != nil || payload.AudioContent == "" {
			return false
		}
		decoded, err := base64.StdEncoding.DecodeString(payload.AudioContent)
		if err != nil {
			return false
		}
		audio = decoded
	}
	format := "mp3"
	if providerID == "coqui" || providerID == "tortoise" {
		format = "wav"
	}
	writeSpeechAudio(w, input, audio, format)
	return true
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
