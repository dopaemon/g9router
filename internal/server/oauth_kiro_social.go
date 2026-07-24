package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"g9router/internal/providers"
)

func (s *Server) kiroSocialAuthorizeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider := r.URL.Query().Get("provider")
	if provider != "google" && provider != "github" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid provider. Use 'google' or 'github'"})
		return
	}
	verifier, err := randomURLToken(32)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	state, err := randomURLToken(24)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	idp := "Google"
	if provider == "github" {
		idp = "Github"
	}
	redirect := "kiro://kiro.kiroAgent/authenticate-success"
	authURL := "https://prod.us-east-1.auth.desktop.kiro.dev/login?idp=" + url.QueryEscape(idp) + "&redirect_uri=" + url.QueryEscape(redirect) + "&code_challenge=" + url.QueryEscape(challenge) + "&code_challenge_method=S256&state=" + url.QueryEscape(state) + "&prompt=select_account"
	writeJSON(w, http.StatusOK, map[string]string{"authUrl": authURL, "state": state, "codeVerifier": verifier, "codeChallenge": challenge, "provider": provider})
}

func (s *Server) kiroSocialExchangeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Code        string `json:"code"`
		CodeVerifier string `json:"codeVerifier"`
		Provider    string `json:"provider"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Code) == "" || strings.TrimSpace(input.CodeVerifier) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing required fields"})
		return
	}
	if input.Provider != "google" && input.Provider != "github" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid provider"})
		return
	}
	body, _ := json.Marshal(map[string]string{"code": strings.TrimSpace(input.Code), "code_verifier": strings.TrimSpace(input.CodeVerifier), "redirect_uri": "kiro://kiro.kiroAgent/authenticate-success"})
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://prod.us-east-1.auth.desktop.kiro.dev/oauth/token", strings.NewReader(string(body)))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Token exchange failed"})
		return
	}
	var token struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ProfileARN   string `json:"profileArn"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if json.Unmarshal(data, &token) != nil || token.AccessToken == "" || token.RefreshToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid token response"})
		return
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	claims := cursorTokenClaims(token.AccessToken)
	email := ""
	if claims != nil {
		if value, ok := claims["email"].(string); ok {
			email = value
		} else if value, ok := claims["sub"].(string); ok {
			email = value
		}
	}
	if err := s.store.Upsert(providers.Provider{ID: "kiro", Name: "Kiro", BaseURL: "https://runtime.us-east-1.kiro.dev", APIKey: token.AccessToken, APIType: "kiro", OAuthID: "kiro", Enabled: true, TestStatus: "active", ProviderSpecificData: map[string]any{"profileArn": token.ProfileARN, "authMethod": input.Provider, "provider": strings.ToUpper(input.Provider[:1]) + input.Provider[1:], "email": email}, Accounts: []providers.Account{{ID: "kiro-social", APIKey: token.AccessToken, Enabled: true}}}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "kiro", "email": email})
}

func randomURLToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(data), "="), nil
}
