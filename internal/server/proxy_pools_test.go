package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyPoolCRUD(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	create := httptest.NewRequest(http.MethodPost, "/api/proxy-pools", strings.NewReader(`{"name":"local","proxyUrl":"http://127.0.0.1:8080","type":"http"}`))
	created := httptest.NewRecorder()
	app.Handler().ServeHTTP(created, create)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"proxyUrl":"http://127.0.0.1:8080"`) {
		t.Fatalf("status=%d body=%s", created.Code, created.Body.String())
	}
	list := httptest.NewRecorder()
	app.Handler().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/proxy-pools?isActive=true", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"local"`) {
		t.Fatalf("status=%d body=%s", list.Code, list.Body.String())
	}
}
