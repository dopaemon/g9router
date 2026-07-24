package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

type Manager struct {
	mu      sync.Mutex
	process *exec.Cmd
	url     string
}

func New() *Manager { return &Manager{} }

func (m *Manager) Start(ctx context.Context, port string) (string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process != nil && m.process.Process != nil {
		return m.url, m.process.Process.Pid, nil
	}
	path, err := exec.LookPath("cloudflared")
	if err != nil {
		return "", 0, fmt.Errorf("cloudflared not installed: %w", err)
	}
	cmd := exec.CommandContext(ctx, path, "tunnel", "--url", "http://localhost:"+port, "--no-autoupdate")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", 0, err
	}
	m.process = cmd
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.process == cmd {
			m.process, m.url = nil, ""
		}
		m.mu.Unlock()
	}()
	urlPattern := regexp.MustCompile(`https://[a-z0-9.-]+\.trycloudflare\.com`)
	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if match := urlPattern.FindString(scanner.Text()); match != "" {
				lines <- match
				return
			}
		}
		close(lines)
	}()
	select {
	case publicURL := <-lines:
		if publicURL == "" {
			return "", cmd.Process.Pid, fmt.Errorf("cloudflared exited without public URL")
		}
		m.url = publicURL
		return publicURL, cmd.Process.Pid, nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return "", 0, ctx.Err()
	}
}

func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process == nil || m.process.Process == nil {
		return false
	}
	_ = m.process.Process.Kill()
	m.process, m.url = nil, ""
	return true
}

func (m *Manager) Status() (string, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process == nil || m.process.Process == nil {
		return m.url, 0, false
	}
	return m.url, m.process.Process.Pid, true
}

func ValidateURL(value string) bool { return strings.HasPrefix(value, "https://") }
