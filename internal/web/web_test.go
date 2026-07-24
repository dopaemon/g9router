package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerServesDashboardRoutes(t *testing.T) {
	for _, path := range []string{"/", "/dashboard", "/login"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		Handler().ServeHTTP(recorder, request)
		if recorder.Code == http.StatusMovedPermanently {
			location := recorder.Header().Get("Location")
			t.Logf("path %s redirected to %s", path, location)
			if location == "/" {
				continue
			}
		}
		if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
			t.Fatalf("path %s: status=%d body=%d", path, recorder.Code, recorder.Body.Len())
		}
	}
}
