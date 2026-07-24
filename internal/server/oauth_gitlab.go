package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) gitlabPATAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Token   string `json:"token"`
		BaseURL string `json:"baseUrl"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Personal Access Token is required"})
		return
	}
	base := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if base == "" {
		base = "https://gitlab.com"
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base+"/api/v4/user", nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Private-Token", strings.TrimSpace(input.Token))
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		status := http.StatusUnauthorized
		if response.StatusCode >= 500 {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]string{"error": "GitLab token verification failed"})
		return
	}
	var user struct {
		Username   string `json:"username"`
		Name       string `json:"name"`
		Email      string `json:"email"`
		PublicMail string `json:"public_email"`
	}
	if json.Unmarshal(data, &user) != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid GitLab user response"})
		return
	}
	email := user.Email
	if email == "" {
		email = user.PublicMail
	}
	if err := s.store.Upsert(providers.Provider{
		ID:          "gitlab",
		Name:        "GitLab Duo",
		BaseURL:     base + "/v1",
		APIKey:      strings.TrimSpace(input.Token),
		APIType:     "openai",
		Enabled:     true,
		TestStatus:  "active",
		ProviderSpecificData: map[string]any{
			"username": usernameOr(user.Username, email),
			"email":    email,
			"name":     user.Name,
			"baseUrl":  base,
			"authKind": "personal_access_token",
		},
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "gitlab", "email": email})
}

func usernameOr(username, fallback string) string {
	if username != "" {
		return username
	}
	return fallback
}
