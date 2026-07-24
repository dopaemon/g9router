package pxpipe

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Timestamp string         `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

type Manager struct {
	mu         sync.RWMutex
	loaded     bool
	events     []Event
	install    string
	installing bool
}

func New() *Manager { return &Manager{} }

func (m *Manager) Status() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	installDir := ""
	if root, err := os.UserCacheDir(); err == nil {
		installDir = filepath.Join(root, "g9router", "pxpipe")
	}
	installed := installDir != "" && fileExists(filepath.Join(installDir, "node_modules", "pxpipe-proxy", "package.json"))
	available := installed && fileExists(filepath.Join(installDir, "node_modules", "pxpipe-proxy", "dist", "core", "library.js"))
	return map[string]any{"installed": installed, "loaded": m.loaded, "available": available, "module": "node-bridge", "installPath": installDir}
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

func (m *Manager) Health(ctx context.Context) map[string]any {
	status := m.Status()
	checks := []map[string]any{{"name": "module", "ok": status["available"] == true}}
	if status["available"] != true {
		return map[string]any{"healthy": false, "checks": checks, "error": "PXPIPE module is not installed"}
	}
	installPath, _ := status["installPath"].(string)
	entry := filepath.Join(installPath, "node_modules", "pxpipe-proxy", "dist", "core", "library.js")
	script := `const m=await import(process.argv[1]);if(typeof m.transformAnthropicMessages!=="function")throw new Error("transformAnthropicMessages missing");const b=new TextEncoder().encode(JSON.stringify({model:"claude-fable-5",max_tokens:16,messages:[{role:"user",content:"ping"}]}));const r=await m.transformAnthropicMessages({body:b,model:"claude-fable-5"});if(!r||typeof r.applied!=="boolean")throw new Error("unexpected transform result");`
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", script, entry)
	if output, err := command.CombinedOutput(); err != nil {
		checks[0]["ok"] = false
		checks = append(checks, map[string]any{"name": "transform", "ok": false, "detail": strings.TrimSpace(string(output))})
		return map[string]any{"healthy": false, "checks": checks, "error": err.Error()}
	}
	checks = append(checks, map[string]any{"name": "transform", "ok": true})
	return map[string]any{"healthy": true, "checks": checks, "error": nil}
}

func (m *Manager) Start() map[string]any {
	m.mu.Lock()
	m.loaded = true
	m.mu.Unlock()
	return m.Status()
}

func (m *Manager) Install(ctx context.Context) (map[string]any, error) {
	m.mu.Lock()
	if m.installing {
		m.mu.Unlock()
		return nil, fmt.Errorf("PXPIPE installation already in progress")
	}
	m.installing = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.installing = false; m.mu.Unlock() }()
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "g9router", "pxpipe")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return nil, fmt.Errorf("npm not found on PATH")
	}
	logPath := filepath.Join(dir, "install.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()
	command := exec.CommandContext(ctx, "npm", "install", "pxpipe-proxy@latest", "--no-audit", "--no-fund", "--omit=dev")
	command.Dir, command.Stdout, command.Stderr = dir, logFile, logFile
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("npm install failed: %w", err)
	}
	m.mu.Lock()
	m.install = logPath
	m.mu.Unlock()
	return map[string]any{"installed": true, "path": dir, "installLog": logPath, "output": strings.TrimSpace(logPath)}, nil
}

func (m *Manager) Stop() bool {
	m.mu.Lock()
	wasLoaded := m.loaded
	m.loaded = false
	m.mu.Unlock()
	return wasLoaded
}

func (m *Manager) AddEvent(data map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, Event{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Data: data})
	if len(m.events) > 1000 {
		m.events = m.events[len(m.events)-1000:]
	}
}

func (m *Manager) Events(limit int) []Event {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit < 1 || limit > len(m.events) {
		limit = len(m.events)
	}
	result := append([]Event(nil), m.events[len(m.events)-limit:]...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}
