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
