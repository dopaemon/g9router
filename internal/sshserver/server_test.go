package sshserver

import (
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"g9router/internal/server"
	gossh "golang.org/x/crypto/ssh"
)

func TestNewRequiresG9RouterPassword(t *testing.T) {
	app := server.New(server.Options{
		ProviderPath: t.TempDir() + "/providers.json",
		OAuthPath:    t.TempDir() + "/oauth.json",
		DatabasePath: t.TempDir() + "/settings.db",
	})
	if _, err := New(app, ":0", "http://127.0.0.1:20128"); err == nil {
		t.Fatal("expected SSH setup to require a configured password")
	}
}

func TestSSHMenuOpensEndpointTUI(t *testing.T) {
	hostKey := t.TempDir() + "/host_key"
	t.Setenv("G9ROUTER_SSH_HOST_KEY", hostKey)
	app := server.New(server.Options{
		ProviderPath: t.TempDir() + "/providers.json",
		OAuthPath:    t.TempDir() + "/oauth.json",
		DatabasePath: t.TempDir() + "/settings.db",
	})
	if err := app.SetPassword("test-password"); err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(app.Handler())
	defer httpServer.Close()
	sshServer, err := New(app, "127.0.0.1:0", httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { _ = sshServer.Serve(listener) }()

	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User:            "dora",
		Auth:            []gossh.AuthMethod{gossh.Password("test-password")},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.RequestPty("xterm", 100, 40, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	input, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := session.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHUntil(output, "Endpoint & Key", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(input, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHUntil(output, "Auto refresh", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(input, "q"); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHUntil(output, "Providers", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(input, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHUntil(output, "Auto refresh", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(input, "\x1b"); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHUntil(output, "Providers", 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func readSSHUntil(reader io.Reader, want string, timeout time.Duration) (string, error) {
	result := make([]byte, 0, 4096)
	buffer := make([]byte, 1024)
	deadline := time.After(timeout)
	for {
		read := make(chan struct{})
		var count int
		var err error
		go func() {
			count, err = reader.Read(buffer)
			close(read)
		}()
		select {
		case <-read:
			result = append(result, buffer[:count]...)
			if strings.Contains(string(result), want) {
				return string(result), nil
			}
			if err != nil {
				return string(result), err
			}
		case <-deadline:
			return string(result), fmt.Errorf("timeout waiting for %q; output=%q", want, string(result))
		}
	}
}
