package headroom

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
)

type Manager struct {
	mu      sync.Mutex
	command string
	process *exec.Cmd
}

func New(command string) *Manager {
	if command == "" {
		command = "headroom"
	}
	return &Manager{command: command}
}
func (m *Manager) Start(ctx context.Context, port int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process != nil && m.process.Process != nil {
		return m.process.Process.Pid, nil
	}
	cmd := exec.CommandContext(ctx, m.command, "--port", strconv.Itoa(port))
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start headroom: %w", err)
	}
	m.process = cmd
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if m.process == cmd {
			m.process = nil
		}
		m.mu.Unlock()
	}()
	return cmd.Process.Pid, nil
}
func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process == nil || m.process.Process == nil {
		return false
	}
	_ = m.process.Process.Kill()
	m.process = nil
	return true
}
func (m *Manager) PID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process == nil || m.process.Process == nil {
		return 0
	}
	return m.process.Process.Pid
}
