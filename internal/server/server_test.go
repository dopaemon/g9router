package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewUsesConfiguredDatabasePath(t *testing.T) {
	path := t.TempDir() + "/state.db"
	app := New(Options{DatabasePath: path})
	if app.database == nil {
		t.Fatal("database is nil")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database path: %v", err)
	}
	if err := app.database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
}

func TestHandlerServesWebUIOnlyWhenEnabled(t *testing.T) {
	for _, test := range []struct {
		name string
		web  bool
		want int
	}{
		{name: "disabled", want: http.StatusNotFound},
		{name: "enabled", web: true, want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := New(Options{
				WebUI:             test.web,
				DatabasePath:      t.TempDir() + "/state.db",
				ProviderPath:      t.TempDir() + "/providers.json",
				OAuthPath:         t.TempDir() + "/oauth.json",
				ProviderNodesPath: t.TempDir() + "/nodes.json",
			})
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			if response.Code != test.want {
				t.Fatalf("status=%d, want %d", response.Code, test.want)
			}
		})
	}
}
