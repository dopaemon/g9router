package sshserver

import (
	"fmt"
	"os"

	"g9router/internal/cli/tui"
	"g9router/internal/server"
	"github.com/charmbracelet/keygen"
	"github.com/charmbracelet/ssh"
)

func New(app *server.Server, addr, baseURL string) (*ssh.Server, error) {
	if !app.PasswordConfigured() {
		return nil, fmt.Errorf("--ssh requires a configured G9Router password")
	}
	hostKeyPath := os.Getenv("G9ROUTER_SSH_HOST_KEY")
	if hostKeyPath == "" {
		hostKeyPath = "id_ed25519"
	}
	if _, err := os.Stat(hostKeyPath); os.IsNotExist(err) {
		if _, err := keygen.New(hostKeyPath, keygen.WithKeyType(keygen.Ed25519), keygen.WithWrite()); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	server := &ssh.Server{
		Addr: addr,
		Handler: func(session ssh.Session) {
			if err := tui.RunAuthenticated(baseURL, session, session); err != nil {
				_, _ = fmt.Fprintln(session.Stderr(), err)
			}
		},
		PasswordHandler: func(_ ssh.Context, password string) bool {
			return app.ValidatePassword(password)
		},
		PublicKeyHandler: func(_ ssh.Context, _ ssh.PublicKey) bool { return false },
	}
	if err := ssh.HostKeyFile(hostKeyPath)(server); err != nil {
		return nil, err
	}
	return server, nil
}
