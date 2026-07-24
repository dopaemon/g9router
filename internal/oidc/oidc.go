package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Config struct{ Issuer, ClientID, ClientSecret, RedirectURL string }
type discovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

func ConfigFromEnv(get func(string) string) Config {
	return Config{Issuer: strings.TrimRight(get("G9ROUTER_OIDC_ISSUER"), "/"), ClientID: get("G9ROUTER_OIDC_CLIENT_ID"), ClientSecret: get("G9ROUTER_OIDC_CLIENT_SECRET"), RedirectURL: get("G9ROUTER_OIDC_REDIRECT_URL")}
}
func (c Config) Enabled() bool { return c.Issuer != "" && c.ClientID != "" && c.RedirectURL != "" }
func (c Config) Discovery(ctx context.Context) (discovery, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return discovery{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return discovery{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return discovery{}, fmt.Errorf("oidc discovery status %s", response.Status)
	}
	var result discovery
	err = json.NewDecoder(response.Body).Decode(&result)
	return result, err
}
func (c Config) AuthorizationURL(ctx context.Context, state string) (string, error) {
	metadata, err := c.Discovery(ctx)
	if err != nil {
		return "", err
	}
	values := url.Values{"response_type": {"code"}, "client_id": {c.ClientID}, "redirect_uri": {c.RedirectURL}, "scope": {"openid profile email"}, "state": {state}}
	return metadata.AuthorizationEndpoint + "?" + values.Encode(), nil
}
func (c Config) Exchange(ctx context.Context, code string) (map[string]any, error) {
	metadata, err := c.Discovery(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "redirect_uri": {c.RedirectURL}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("oidc token status %s: %s", response.Status, body)
	}
	var result map[string]any
	err = json.Unmarshal(body, &result)
	return result, err
}
func NewState() string {
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	return base64.RawURLEncoding.EncodeToString(raw)
}
