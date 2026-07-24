package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type edgeToken struct {
	Key, Token, Cookie string
}

var edgeTokenCache struct {
	sync.Mutex
	token edgeToken
	at    time.Time
}

var edgeTokenPattern = regexp.MustCompile(`params_AbusePreventionHelper\s*=\s*\[([^,]+),([^,]+),`)

func edgeVoiceModel(model string) (string, bool) {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(model, "edge-tts/") {
		return strings.TrimPrefix(model, "edge-tts/"), true
	}
	return model, model == "edge-tts"
}

func (s *Server) edgeTTSAPI(w http.ResponseWriter, r *http.Request, input map[string]any) bool {
	model, _ := input["model"].(string)
	voice, ok := edgeVoiceModel(model)
	if !ok {
		return false
	}
	text, _ := input["input"].(string)
	if strings.TrimSpace(text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing required field: input"})
		return true
	}
	if voice == "" || voice == "edge-tts" {
		voice = "vi-VN-HoaiMyNeural"
	}
	token, err := s.edgeToken(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return true
	}
	audio, err := s.edgeSynthesize(r.Context(), text, voice, token)
	if err != nil {
		edgeTokenCache.Lock()
		edgeTokenCache.at = time.Time{}
		edgeTokenCache.Unlock()
		token, refreshErr := s.edgeToken(r.Context())
		if refreshErr == nil {
			audio, err = s.edgeSynthesize(r.Context(), text, voice, token)
		}
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return true
	}
	if r.URL.Query().Get("response_format") == "json" || input["response_format"] == "json" {
		writeJSON(w, http.StatusOK, map[string]string{"audio": string(audio), "format": "mp3"})
		return true
	}
	w.Header().Set("Content-Type", "audio/mp3")
	w.Header().Set("Content-Length", fmt.Sprint(len(audio)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
	return true
}

func (s *Server) edgeToken(ctx context.Context) (edgeToken, error) {
	edgeTokenCache.Lock()
	if edgeTokenCache.token.Token != "" && time.Since(edgeTokenCache.at) < 5*time.Minute {
		token := edgeTokenCache.token
		edgeTokenCache.Unlock()
		return token, nil
	}
	edgeTokenCache.Unlock()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.bing.com/translator", nil)
	if err != nil {
		return edgeToken{}, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	request.Header.Set("Accept-Language", "vi,en-US;q=0.9,en;q=0.8")
	response, err := s.client.Do(request)
	if err != nil {
		return edgeToken{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return edgeToken{}, fmt.Errorf("Bing translator fetch failed: %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return edgeToken{}, err
	}
	match := edgeTokenPattern.FindSubmatch(body)
	if len(match) < 3 {
		return edgeToken{}, fmt.Errorf("failed to parse Bing token")
	}
	cookie := response.Header.Get("Set-Cookie")
	token := edgeToken{Key: strings.TrimSpace(string(match[1])), Token: strings.Trim(strings.TrimSpace(string(match[2])), `"`), Cookie: strings.Split(cookie, ";")[0]}
	edgeTokenCache.Lock()
	edgeTokenCache.token, edgeTokenCache.at = token, time.Now()
	edgeTokenCache.Unlock()
	return token, nil
}

func (s *Server) edgeSynthesize(ctx context.Context, text, voice string, token edgeToken) ([]byte, error) {
	parts := strings.Split(voice, "-")
	lang := "en-US"
	if len(parts) >= 2 {
		lang = strings.Join(parts[:2], "-")
	}
	gender := "Female"
	if strings.Contains(strings.ToLower(voice), "male") {
		gender = "Male"
	}
	ssml := fmt.Sprintf("<speak version='1.0' xml:lang='%s'><voice xml:lang='%s' xml:gender='%s' name='%s'><prosody rate='0.00%%'>%s</prosody></voice></speak>", lang, lang, gender, voice, escapeXML(text))
	form := url.Values{"ssml": {ssml}, "token": {token.Token}, "key": {token.Key}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.bing.com/tfettts?isVertical=1&&IG=1&IID=translator.5023&SFX=1", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "https://www.bing.com")
	request.Header.Set("Referer", "https://www.bing.com/translator")
	request.Header.Set("User-Agent", "Mozilla/5.0")
	if token.Cookie != "" {
		request.Header.Set("Cookie", token.Cookie)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Bing TTS failed: %s", response.Status)
	}
	if len(data) < 1024 {
		return nil, fmt.Errorf("Bing TTS returned empty audio")
	}
	return data, nil
}

func escapeXML(value string) string {
	data, _ := json.Marshal(value)
	var decoded string
	_ = json.Unmarshal(data, &decoded)
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;").Replace(decoded)
}
