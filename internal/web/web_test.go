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

func TestHandlerServesFavicon(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/favicon.svg", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<svg") {
		t.Fatalf("favicon status=%d body=%d", response.Code, response.Body.Len())
	}
}

func TestHandlerServesManifest(t *testing.T) {
	response := httptest.NewRecorder()
	Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manifest.json", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"short_name":"9Router"`) {
		t.Fatalf("manifest status=%d body=%d", response.Code, response.Body.Len())
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
	body := response.Body.String()
	for _, marker := range []string{"cliGuideMetadata", "cliGuideContent", "guideSteps", "codeBlock"} {
		if !strings.Contains(body, marker) {
			t.Fatalf("dashboard missing CLI guide marker %q", marker)
		}
	}
}
