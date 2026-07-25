package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUsageDetailsValidation(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	bad := httptest.NewRecorder()
	app.Handler().ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/usage/chart?period=bad", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("chart status=%d body=%s", bad.Code, bad.Body.String())
	}
	details := httptest.NewRecorder()
	app.Handler().ServeHTTP(details, httptest.NewRequest(http.MethodGet, "/api/usage/request-details?page=0", nil))
	if details.Code != http.StatusBadRequest || !strings.Contains(details.Body.String(), "Page must be") {
		t.Fatalf("details status=%d body=%s", details.Code, details.Body.String())
	}
}

func TestUsageHistoryUsesAllPeriod(t *testing.T) {
	app := New(Options{DatabasePath: t.TempDir() + "/test.db"})
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/usage/history", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"totalRequests"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUsageChartUsesHourlyBuckets(t *testing.T) {
	app := New(Options{DatabasePath: t.TempDir() + "/test.db"})
	for _, period := range []string{"today", "24h"} {
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/usage/chart?period="+period, nil))
		if response.Code != http.StatusOK || strings.Count(response.Body.String(), `"label"`) != 24 {
			t.Fatalf("period=%s status=%d body=%s", period, response.Code, response.Body.String())
		}
	}
}
