package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Server struct {
	upstream        *http.Client
	baseURL, apiKey string
}

func main() {
	addr := getenv("G9ROUTER_ADDR", ":20128")
	server := NewServer(getenv("G9ROUTER_UPSTREAM", "https://api.openai.com/v1"), os.Getenv("G9ROUTER_API_KEY"))
	log.Printf("g9router listening on %s, upstream %s", addr, server.baseURL)
	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}

func NewServer(baseURL, apiKey string) *Server {
	return &Server{upstream: &http.Client{Timeout: 10 * time.Minute}, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/v1/models", s.models)
	mux.HandleFunc("/v1/chat/completions", s.chatCompletions)
	return logging(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	s.proxy(w, r, "/models", http.MethodGet, nil)
}

func (s *Server) chatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
		return
	}
	var request struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	s.proxy(w, r, "/chat/completions", http.MethodPost, body)
}

func (s *Server) proxy(w http.ResponseWriter, incoming *http.Request, path, method string, body []byte) {
	ctx, cancel := context.WithTimeout(incoming.Context(), 10*time.Minute)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Accept", incoming.Header.Get("Accept"))
	request.Header.Set("Content-Type", "application/json")
	if key := incoming.Header.Get("Authorization"); key != "" {
		request.Header.Set("Authorization", key)
	} else if s.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	response, err := s.upstream.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if response.Header.Get("Content-Type") == "text/event-stream" {
		if flusher, ok := w.(http.Flusher); ok {
			streamCopy(w, response.Body, flusher)
			return
		}
	}
	_, _ = io.Copy(w, response.Body)
}

func streamCopy(w io.Writer, body io.Reader, flusher http.Flusher) {
	buffer := make([]byte, 32<<10)
	for {
		count, err := body.Read(buffer)
		if count > 0 {
			_, _ = w.Write(buffer[:count])
			flusher.Flush()
		}
		if errors.Is(err, io.EOF) || err != nil {
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
	})
}
func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
