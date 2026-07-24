package mcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type Session struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  <-chan string
	done    chan struct{}
}
type Bridge struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func New() *Bridge { return &Bridge{sessions: map[string]*Session{}} }
func (b *Bridge) Start(ctx context.Context, command string) (string, <-chan string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("empty MCP command")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	input, err := cmd.StdinPipe()
	if err != nil {
		return "", nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, err
	}
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	lines := make(chan string, 16)
	session := &Session{command: cmd, input: input, output: lines, done: make(chan struct{})}
	id := sessionID()
	b.mu.Lock()
	b.sessions[id] = session
	b.mu.Unlock()
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 16<<20)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(session.done)
		b.mu.Lock()
		delete(b.sessions, id)
		b.mu.Unlock()
	}()
	return id, lines, nil
}
func (b *Bridge) Send(id string, value any) error {
	b.mu.Lock()
	session, ok := b.sessions[id]
	b.mu.Unlock()
	if !ok {
		return fmt.Errorf("MCP session not found")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(session.input, string(encoded))
	return err
}
func (b *Bridge) Close(id string) {
	b.mu.Lock()
	session := b.sessions[id]
	delete(b.sessions, id)
	b.mu.Unlock()
	if session != nil {
		_ = session.input.Close()
		_ = session.command.Process.Kill()
	}
}
func sessionID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "session"
	}
	return hex.EncodeToString(raw)
}
