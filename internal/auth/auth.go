package auth

import (
	"crypto/subtle"
	"net/http"
	"sync"
	"time"
)

type Sessions struct {
	mu     sync.RWMutex
	values map[string]time.Time
}

func NewSessions() *Sessions { return &Sessions{values: map[string]time.Time{}} }
func (s *Sessions) Create(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[token] = time.Now().Add(24 * time.Hour)
}
func (s *Sessions) Valid(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expiry, ok := s.values[token]
	return ok && time.Now().Before(expiry)
}
func (s *Sessions) Delete(token string) { s.mu.Lock(); defer s.mu.Unlock(); delete(s.values, token) }

func Middleware(next http.Handler, key string) http.Handler {
	if key == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/auth/status" || r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/logout" {
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
