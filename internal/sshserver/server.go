package sshserver

import (
	"fmt"
	"os"

	"g9router/internal/cli/tui"
	"g9router/internal/server"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
)

func New(app *server.Server, addr, baseURL string) (*ssh.Server, error) {
	if !app.PasswordConfigured() {
		return nil, fmt.Errorf("--ssh requires a configured G9Router password")
	}
	hostKeyPath := os.Getenv("G9ROUTER_SSH_HOST_KEY")
	if hostKeyPath == "" {
		hostKeyPath = "id_ed25519"
	}
	return wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithPasswordAuth(func(_ ssh.Context, password string) bool {
			return app.ValidatePassword(password)
		}),
		wish.WithPublicKeyAuth(func(_ ssh.Context, _ ssh.PublicKey) bool { return false }),
		wish.WithMiddleware(func(next ssh.Handler) ssh.Handler {
			return func(session ssh.Session) {
				if err := tui.RunAuthenticated(baseURL, session, session); err != nil {
					_, _ = fmt.Fprintln(session.Stderr(), err)
				}
			}
		}),
	)
}
