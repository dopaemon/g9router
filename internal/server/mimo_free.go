package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"strings"
	"sync"
	"time"
)

const mimoSystemMarker = "You are MiMoCode, an interactive CLI tool that helps users with software engineering tasks."

var mimoJWT struct {
	sync.Mutex
	value string
	exp   time.Time
}

func (s *Server) mimoFreeChat(w http.ResponseWriter, r *http.Request, body []byte, request map[string]any) bool {
	jwt, err := s.mimoJWT(r)
	if err != nil {
		return false
	}
	messages, _ := request["messages"].([]any)
	hasMarker := false
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "system" && strings.Contains(stringValue(message["content"]), mimoSystemMarker) {
			hasMarker = true
		}
	}
	if !hasMarker {
		request["messages"] = append([]any{map[string]any{"role": "system", "content": mimoSystemMarker}}, messages...)
	}
	request["stream"] = true
	encoded, _ := json.Marshal(request)
	doRequest := func(token string) (*http.Response, error) {
		upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.xiaomimimo.com/api/free-ai/openai/chat", strings.NewReader(string(encoded)))
		if err != nil {
			return nil, err
		}
		upstream.Header.Set("Content-Type", "application/json")
		upstream.Header.Set("Authorization", "Bearer "+token)
		upstream.Header.Set("X-Mimo-Source", "mimocode-cli-free")
		upstream.Header.Set("User-Agent", "Mozilla/5.0 Chrome/131.0.0.0")
		upstream.Header.Set("x-session-affinity", "ses_"+uuidToken(string(body))[:24])
		upstream.Header.Set("Accept", "text/event-stream")
		return s.client.Do(upstream)
	}
	response, err := doRequest(jwt)
	if err != nil {
		return false
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		response.Body.Close()
		mimoJWT.Lock()
		mimoJWT.value, mimoJWT.exp = "", time.Time{}
		mimoJWT.Unlock()
		jwt, err = s.mimoJWT(r)
		if err != nil {
			return false
		}
		response, err = doRequest(jwt)
	}
	if err != nil || response.StatusCode >= 500 {
		if response != nil {
			response.Body.Close()
		}
		return false
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		if key == "Content-Length" || key == "Content-Encoding" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
	return true
}

func (s *Server) mimoJWT(r *http.Request) (string, error) {
	mimoJWT.Lock()
	if mimoJWT.value != "" && time.Now().Before(mimoJWT.exp.Add(-5*time.Minute)) {
		value := mimoJWT.value
		mimoJWT.Unlock()
		return value, nil
	}
	mimoJWT.Unlock()
	host, _ := os.Hostname()
	name := "unknown-user"
	if current, err := user.Current(); err == nil {
		name = current.Username
	}
	seed := fmt.Sprintf("%s|%s|%s|%s", host, os.GOOS, os.Getenv("GOARCH"), name)
	hash := sha256.Sum256([]byte(seed))
	body, _ := json.Marshal(map[string]string{"client": fmt.Sprintf("%x", hash[:])})
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.xiaomimimo.com/api/free-ai/bootstrap", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Mozilla/5.0 Chrome/131.0.0.0")
	response, err := s.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var payload struct {
		JWT string `json:"jwt"`
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload) != nil || payload.JWT == "" {
		return "", fmt.Errorf("mimo bootstrap failed: %s", response.Status)
	}
	exp := time.Now().Add(50 * time.Minute)
	parts := strings.Split(payload.JWT, ".")
	if len(parts) == 3 {
		if data, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
			var claims struct {
				Exp int64 `json:"exp"`
			}
			if json.Unmarshal(data, &claims) == nil && claims.Exp > 0 {
				exp = time.Unix(claims.Exp, 0)
			}
		}
	}
	mimoJWT.Lock()
	mimoJWT.value, mimoJWT.exp = payload.JWT, exp
	mimoJWT.Unlock()
	return payload.JWT, nil
}
