package server

import (
	"encoding/json"
	"net/http"
	"sync"
)

var consoleLogBuffer struct {
	sync.RWMutex
	lines []string
	subs  map[chan string]struct{}
}

func addConsoleLog(line string) {
	consoleLogBuffer.Lock()
	defer consoleLogBuffer.Unlock()
	if consoleLogBuffer.subs == nil {
		consoleLogBuffer.subs = map[chan string]struct{}{}
	}
	consoleLogBuffer.lines = append(consoleLogBuffer.lines, line)
	if len(consoleLogBuffer.lines) > 500 {
		consoleLogBuffer.lines = consoleLogBuffer.lines[len(consoleLogBuffer.lines)-500:]
	}
	for subscriber := range consoleLogBuffer.subs {
		select {
		case subscriber <- line:
		default:
		}
	}
}

func (s *Server) consoleLogsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		consoleLogBuffer.Lock()
		consoleLogBuffer.lines = nil
		consoleLogBuffer.Unlock()
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	consoleLogBuffer.RLock()
	lines := append([]string(nil), consoleLogBuffer.lines...)
	consoleLogBuffer.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "logs": lines})
}

func (s *Server) consoleLogsStreamAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	updates := make(chan string, 16)
	consoleLogBuffer.Lock()
	if consoleLogBuffer.subs == nil {
		consoleLogBuffer.subs = map[chan string]struct{}{}
	}
	consoleLogBuffer.subs[updates] = struct{}{}
	initial := append([]string(nil), consoleLogBuffer.lines...)
	consoleLogBuffer.Unlock()
	defer func() {
		consoleLogBuffer.Lock()
		delete(consoleLogBuffer.subs, updates)
		close(updates)
		consoleLogBuffer.Unlock()
	}()
	if len(initial) > 0 {
		data, _ := json.Marshal(map[string]any{"type": "init", "logs": initial})
		_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case line := <-updates:
			data, _ := json.Marshal(map[string]any{"type": "line", "line": line})
			if _, err := w.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
