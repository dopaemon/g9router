package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslatorFileLoadSave(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	save := httptest.NewRecorder()
	app.Handler().ServeHTTP(save, httptest.NewRequest(http.MethodPost, "/api/translator/save", strings.NewReader(`{"file":"1_req_client.json","content":"{}"}`)))
	if save.Code != http.StatusOK {
		t.Fatalf("save status=%d body=%s", save.Code, save.Body.String())
	}
	load := httptest.NewRecorder()
	app.Handler().ServeHTTP(load, httptest.NewRequest(http.MethodGet, "/api/translator/load?file=1_req_client.json", nil))
	if load.Code != http.StatusOK || !strings.Contains(load.Body.String(), `"content":"{}"`) {
		t.Fatalf("load status=%d body=%s", load.Code, load.Body.String())
	}
}
