package auth

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
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
	return MiddlewareWithValidator(next, key, nil, false)
}

func MiddlewareWithValidator(next http.Handler, key string, validator func(string) bool, enabled bool) http.Handler {
	return middleware(next, key, validator, enabled, nil)
}

func MiddlewareWithSession(next http.Handler, key string, validator func(string) bool, enabled bool, sessions *Sessions) http.Handler {
	return middleware(next, key, validator, enabled, sessions)
}

func middleware(next http.Handler, key string, validator func(string) bool, enabled bool, sessions *Sessions) http.Handler {
	if key == "" && !enabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/auth/status" || r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-G9Router-Local-CLI") == "1" && isLoopback(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		provided := r.Header.Get("X-G9Router-Key")
		if provided == "" {
			provided = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		}
		valid := key != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(key)) == 1
		if !valid && validator != nil {
			valid = validator(provided)
		}
		if !valid && sessions != nil {
			if cookie, err := r.Cookie("g9router_session"); err == nil {
				valid = sessions.Valid(cookie.Value)
			}
		}
		if !valid {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
