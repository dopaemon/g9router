package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"g9router/internal/oauth"
	"g9router/internal/providers"
)

func (s *Server) kiroExternalImportAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var body map[string]any
	if json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CLIProxyAPI auth JSON is invalid"})
		return
	}
	raw := body
	for _, key := range []string{"cliProxyAuth", "auth", "json"} {
		if nested, ok := body[key].(map[string]any); ok {
			raw = nested
			break
		}
	}
	get := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := raw[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	if method := get("auth_method", "authMethod"); method != "" && method != "external_idp" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Only external_idp Kiro auth is supported by this importer"})
		return
	}
	accessToken, refreshToken, clientID := get("access_token", "accessToken"), get("refresh_token", "refreshToken"), get("client_id", "clientId")
	tokenEndpoint := get("token_endpoint", "tokenEndpoint")
	parsed, err := url.Parse(tokenEndpoint)
	if err != nil || parsed.Scheme != "https" || !map[string]bool{"login.microsoftonline.com": true, "login.microsoft.com": true, "login.windows.net": true}[strings.ToLower(parsed.Hostname())] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token_endpoint must be a Microsoft login endpoint"})
		return
	}
	profileARN, region, scope := get("profile_arn", "profileArn"), get("region"), normalizeExternalScope(raw["scopes"], raw["scope"])
	if accessToken == "" || refreshToken == "" || clientID == "" || scope == "" || profileARN == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "access_token, refresh_token, client_id, scopes and profile_arn are required"})
		return
	}
	if region == "" {
		region = "us-east-1"
	}
	email := get("email")
	if email == "" {
		if claims := cursorTokenClaims(accessToken); claims != nil {
			for _, key := range []string{"email", "preferred_username", "upn", "sub"} {
				if value, ok := claims[key].(string); ok && value != "" {
					email = value
					break
				}
			}
		}
	}
	expiresAt := externalExpiresAt(raw, accessToken)
	providerData := map[string]any{"profileArn": profileARN, "region": region, "authMethod": "external_idp", "provider": "CLIProxyAPI", "clientId": clientID, "tokenEndpoint": parsed.String(), "scope": scope, "refreshToken": refreshToken, "expiresAt": expiresAt, "email": email}
	credentialID := "kiro-external-idp"
	if err := s.oauth.Upsert(oauth.Credential{ID: credentialID, Provider: "kiro", AccessToken: accessToken, RefreshToken: refreshToken, TokenURL: parsed.String(), ClientID: clientID, ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := s.store.Upsert(providers.Provider{ID: "kiro", Name: "Kiro", BaseURL: "https://runtime." + region + ".kiro.dev", APIKey: accessToken, APIType: "kiro", OAuthID: credentialID, Enabled: true, TestStatus: "active", ProviderSpecificData: providerData}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "provider": "kiro", "email": email, "expiresAt": expiresAt, "connection": map[string]string{"provider": "kiro", "id": credentialID}})
}

func normalizeExternalScope(values ...any) string {
	parts := []string{}
	for _, value := range values {
		switch item := value.(type) {
		case string:
			if strings.TrimSpace(item) != "" {
				parts = append(parts, strings.TrimSpace(item))
			}
		case []any:
			for _, child := range item {
				if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
				}
			}
		}
		if len(parts) > 0 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func externalExpiresAt(input map[string]any, token string) string {
	for _, key := range []string{"expired", "expires_at", "expiresAt"} {
		if value, ok := input[key].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				return parsed.UTC().Format(time.RFC3339)
			}
		}
	}
	if value, ok := input["expires_in"].(float64); ok && value > 0 {
		return time.Now().Add(time.Duration(value) * time.Second).UTC().Format(time.RFC3339)
	}
	parts := strings.Split(token, ".")
	if len(parts) == 3 {
		payload := parts[1] + strings.Repeat("=", (4-len(parts[1])%4)%4)
		if data, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(payload, "=")); err == nil {
			var claims map[string]any
			if json.Unmarshal(data, &claims) == nil {
				if exp, ok := claims["exp"].(float64); ok {
					return time.Unix(int64(exp), 0).UTC().Format(time.RFC3339)
				}
			}
		}
	}
	return time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
}
