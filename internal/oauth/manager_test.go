package oauth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshPersistsAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"new-token","expires_in":3600}`)
	}))
	defer server.Close()
	manager := New(t.TempDir() + "/oauth.json")
	if err := manager.Upsert(Credential{ID: "demo", RefreshToken: "refresh", TokenURL: server.URL, ClientID: "client"}); err != nil {
		t.Fatal(err)
	}
	credential, err := manager.Refresh(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != "new-token" || credential.ExpiresAt == 0 {
		t.Fatal(credential)
	}
}

func TestRefreshSupportsKiroCamelCase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"accessToken":"kiro-token","refreshToken":"next-refresh","expiresIn":3600}`)
	}))
	defer server.Close()
	manager := New(t.TempDir() + "/kiro.json")
	if err := manager.Upsert(Credential{ID: "kiro", RefreshToken: "refresh", TokenURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	credential, err := manager.Refresh(t.Context(), "kiro")
	if err != nil || credential.AccessToken != "kiro-token" || credential.RefreshToken != "next-refresh" {
		t.Fatal(credential, err)
	}
}
