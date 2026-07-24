package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/oidc"
)

func (s *Server) oidcTestAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input map[string]any
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input)
	settings := s.settings.Get()
	stringValue := func(key string) string {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		if value, ok := settings[key].(string); ok {
			return strings.TrimSpace(value)
		}
		return ""
	}
	issuer, clientID := stringValue("issuerUrl"), stringValue("clientId")
	scopes := stringValue("scopes")
	if scopes == "" {
		scopes = "openid profile email"
	}
	if issuer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Issuer URL is required"})
		return
	}
	if clientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Client ID is required"})
		return
	}
	config := oidc.Config{Issuer: strings.TrimRight(issuer, "/"), ClientID: clientID}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	discovery, err := config.Discovery(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	origin := "http://" + r.Host
	if r.TLS != nil {
		origin = "https://" + r.Host
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "discoveryOk": true, "clientSecretTested": false, "clientSecretValid": nil, "issuerUrl": issuer, "clientId": clientID, "scopes": scopes, "redirectUri": origin + "/api/auth/oidc/callback", "authorizationEndpoint": discovery.AuthorizationEndpoint, "tokenEndpoint": discovery.TokenEndpoint, "jwksUri": "", "message": "OIDC discovery loaded"})
}
