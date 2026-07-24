package vertex

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}
type AuthorizedUser struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	TokenURI     string `json:"token_uri"`
	QuotaProject string `json:"quota_project_id"`
}

func ParseAuthorizedUser(raw string) (AuthorizedUser, error) {
	var user AuthorizedUser
	if err := json.Unmarshal([]byte(raw), &user); err != nil || user.ClientID == "" || user.ClientSecret == "" || user.RefreshToken == "" {
		return user, fmt.Errorf("invalid authorized user")
	}
	return user, nil
}

func ParseServiceAccount(raw string) (ServiceAccount, error) {
	var account ServiceAccount
	if err := json.Unmarshal([]byte(raw), &account); err != nil {
		return account, err
	}
	if account.ProjectID == "" || account.ClientEmail == "" || account.PrivateKey == "" {
		return account, fmt.Errorf("incomplete service account")
	}
	if account.TokenURI == "" {
		account.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return account, nil
}

func AccessToken(ctx context.Context, client *http.Client, raw string) (string, error) {
	account, err := ParseServiceAccount(raw)
	if err != nil {
		return authorizedUserToken(ctx, client, raw)
	}
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("invalid private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if parsed, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes); parseErr != nil {
			return "", err
		} else {
			key = parsed
		}
	}
	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not RSA")
	}
	now := time.Now()
	header := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iss": account.ClientEmail, "scope": "https://www.googleapis.com/auth/cloud-platform", "aud": account.TokenURI, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	unsigned := header + "." + b64(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, cryptoHash(), digest[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + b64(signature)}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return "", fmt.Errorf("token status %s", response.Status)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}
	return payload.AccessToken, nil
}

func authorizedUserToken(ctx context.Context, client *http.Client, raw string) (string, error) {
	user, err := ParseAuthorizedUser(raw)
	if err != nil {
		return "", err
	}
	if user.TokenURI == "" {
		user.TokenURI = "https://oauth2.googleapis.com/token"
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {user.RefreshToken}, "client_id": {user.ClientID}, "client_secret": {user.ClientSecret}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, user.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return "", fmt.Errorf("token status %s", response.Status)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}
	return payload.AccessToken, nil
}
func b64(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
func cryptoHash() crypto.Hash { return crypto.SHA256 }
