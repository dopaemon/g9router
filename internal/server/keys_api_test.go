package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	keyStore "g9router/internal/keys"
)

func TestKeysAPIListRedactsSecrets(t *testing.T) {
	app := New(Options{})
	app.keys = keyStore.New(t.TempDir() + "/keys.json")
	item, err := app.keys.Create("test", "machine")
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	app.keysAPI(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), item.Key) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
