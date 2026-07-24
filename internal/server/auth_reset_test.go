package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResetPasswordAPI(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	recorder := httptest.NewRecorder()
	app.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/auth/reset-password", strings.NewReader(`{}`)))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "{\"success\":true}\n" {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSettingsPasswordChangeAndLogin(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	change := httptest.NewRecorder()
	app.Handler().ServeHTTP(change, httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(`{"currentPassword":"123456","newPassword":"new-secret"}`)))
	if change.Code != http.StatusOK {
		t.Fatalf("change status=%d body=%s", change.Code, change.Body.String())
	}
	login := httptest.NewRecorder()
	app.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"new-secret"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
}
