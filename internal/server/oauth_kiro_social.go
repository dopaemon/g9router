package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
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

func randomURLToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(data), "="), nil
}
