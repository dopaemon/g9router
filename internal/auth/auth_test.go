package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareRequiresKey(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), "secret")
	request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatal(response.Code)
	}
	request.Header.Set("X-G9Router-Key", "secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal(response.Code)
	}
}

func TestMiddlewareAcceptsValidatorKey(t *testing.T) {
	handler := MiddlewareWithValidator(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), "", func(value string) bool { return value == "generated" }, true)
	request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	request.Header.Set("X-G9Router-Key", "generated")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal(response.Code)
	}
}

func TestMiddlewareAcceptsBearerValidatorKey(t *testing.T) {
	handler := MiddlewareWithValidator(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), "", func(value string) bool { return value == "generated" }, true)
	request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	request.Header.Set("Authorization", "Bearer generated")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal(response.Code)
	}
}

func TestMiddlewareAcceptsSessionCookie(t *testing.T) {
	sessions := NewSessions()
	sessions.Create("session-token")
	handler := MiddlewareWithSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }), "", nil, true, sessions)
	request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	request.AddCookie(&http.Cookie{Name: "g9router_session", Value: "session-token"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal(response.Code)
	}
}

func TestMiddlewareAcceptsLocalCLI(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "required")
	request := httptest.NewRequest(http.MethodGet, "/api/keys", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("X-G9Router-Local-CLI", "1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestMiddlewareLeavesWebShellPublic(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "required")
	for _, path := range []string{"/", "/login", "/dashboard", "/dashboard/providers", "/favicon.svg"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("path %s status = %d", path, response.Code)
		}
	}
}

func TestMiddlewareStillProtectsAPIs(t *testing.T) {
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "required")
	request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("API status = %d", response.Code)
	}
}
