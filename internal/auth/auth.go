package auth

import (
	"crypto/subtle"
	"net/http"
)

func Middleware(next http.Handler, key string) http.Handler {
	if key == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/auth/status" {
			next.ServeHTTP(w, r)
			return
		}
		provided := r.Header.Get("X-G9Router-Key")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
