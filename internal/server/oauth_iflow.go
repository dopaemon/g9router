package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) iflowCookieAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Cookie string `json:"cookie"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || !strings.Contains(input.Cookie, "BXAuth=") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cookie must contain BXAuth field"})
		return
	}
	cookie := strings.TrimSpace(input.Cookie)
	if !strings.HasSuffix(cookie, ";") {
		cookie += ";"
	}
	clientRequest := func(method string, body io.Reader) (*http.Response, error) {
		request, err := http.NewRequestWithContext(r.Context(), method, "https://platform.iflow.cn/api/openapi/apikey", body)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Cookie", cookie)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "Mozilla/5.0")
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/json")
		}
		return s.client.Do(request)
	}
	first, err := clientRequest(http.MethodGet, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	firstData, _ := io.ReadAll(io.LimitReader(first.Body, 2<<20))
	first.Body.Close()
	if first.StatusCode < 200 || first.StatusCode >= 300 {
		writeJSON(w, first.StatusCode, map[string]string{"error": "Failed to fetch API key info"})
		return
	}
	var firstPayload struct {
		Success bool `json:"success"`
		Data    struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(firstData, &firstPayload) != nil || !firstPayload.Success || firstPayload.Data.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "API key info unavailable"})
		return
	}
	body, _ := json.Marshal(map[string]string{"name": firstPayload.Data.Name})
	second, err := clientRequest(http.MethodPost, strings.NewReader(string(body)))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	secondData, _ := io.ReadAll(io.LimitReader(second.Body, 2<<20))
	second.Body.Close()
	if second.StatusCode < 200 || second.StatusCode >= 300 {
		writeJSON(w, second.StatusCode, map[string]string{"error": "Failed to refresh API key"})
		return
	}
	var secondPayload struct {
		Success bool `json:"success"`
		Data    struct {
			Name       string `json:"name"`
			APIKey     string `json:"apiKey"`
			ExpireTime any    `json:"expireTime"`
		} `json:"data"`
	}
	if json.Unmarshal(secondData, &secondPayload) != nil || !secondPayload.Success || secondPayload.Data.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "API key refresh failed"})
		return
	}
	bxAuth := cookie
	if index := strings.Index(bxAuth, "BXAuth="); index >= 0 {
		bxAuth = bxAuth[index:]
		if end := strings.IndexByte(bxAuth, ';'); end >= 0 {
			bxAuth = bxAuth[:end+1]
		}
	}
	if err := s.store.Upsert(providers.Provider{ID: "iflow", Name: "iFlow", BaseURL: "https://apis.iflow.cn/v1", APIKey: secondPayload.Data.APIKey, APIType: "openai", Enabled: true, TestStatus: "active", ProviderSpecificData: map[string]any{"cookie": bxAuth, "expireTime": secondPayload.Data.ExpireTime, "authType": "cookie"}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	masked := secondPayload.Data.APIKey
	if len(masked) > 10 {
		masked = masked[:10] + "..."
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "iflow", "apiKey": masked, "expireTime": secondPayload.Data.ExpireTime})
}
