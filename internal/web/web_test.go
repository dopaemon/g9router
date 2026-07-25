package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesDashboardRoutes(t *testing.T) {
	for _, path := range []string{"/", "/dashboard", "/login"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		Handler().ServeHTTP(recorder, request)
		if recorder.Code == http.StatusMovedPermanently {
			location := recorder.Header().Get("Location")
			if location == "/" {
				continue
			}
		}
		if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
			t.Fatalf("path %s: status=%d body=%d", path, recorder.Code, recorder.Body.Len())
		}
	}
}

func TestDashboardContainsPortedControls(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dashboard/providers/demo", nil))
	body := response.Body.String()
	for _, marker := range []string{"/api/providers/", "/api/models/custom", "/api/oauth/cursor/auto-import", "/api/usage/"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("dashboard missing %q", marker)
		}
	}
}

func TestDashboardInjectsCLIGuideCard(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dashboard/cli-tools", nil))
	if !strings.Contains(response.Body.String(), "cliGuideMetadata") {
		t.Fatal("dashboard missing CLI guide metadata card")
	}
}
