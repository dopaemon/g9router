package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"g9router/internal/auth"
	"g9router/internal/cli/tray"
	"g9router/internal/cli/tui"
	"g9router/internal/cli/xai"
	"g9router/internal/instance"
	"g9router/internal/server"
	"g9router/internal/sshserver"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

func main() {
	tui.EnableColors(os.Stdout)
	if len(os.Args) >= 3 && os.Args[1] == "xai" && os.Args[2] == "video" {
		os.Exit(xai.Run(context.Background(), os.Args[3:], os.Stdout, os.Stderr))
	}
	var options runOptions
	command := &cobra.Command{
		Use:   "g9router",
		Short: "Go-compatible AI gateway",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return run(options) },
	}
	flags := command.Flags()
	flags.IntVarP(&options.port, "port", "p", 0, "port to run the server")
	flags.StringVarP(&options.host, "host", "H", "", "host to bind")
	flags.BoolVarP(&options.noBrowser, "no-browser", "n", false, "do not open the dashboard")
	flags.BoolVar(&options.webUI, "webui", false, "enable the web UI")
	flags.BoolVar(&options.interactive, "interactive", false, "run the interactive Huh CLI")
	flags.BoolVar(&options.log, "log", false, "show server logs")
	flags.BoolVar(&options.tray, "tray", false, "run in system tray mode")
	flags.BoolVar(&options.ssh, "ssh", false, "enable the SSH TUI server")
	flags.IntVar(&options.sshPort, "ssh-port", 2222, "SSH server port")
	flags.StringVar(&options.setPassword, "set-password", "", "set the G9Router password and exit")
	flags.StringVar(&options.resetPassword, "reset-password", "", "reset the G9Router password using the current password and exit")
	flags.BoolVar(&options.skipUpdate, "skip-update", false, "skip update check")
	if err := fang.Execute(context.Background(), command, fang.WithVersion("9Router v0.5.40")); err != nil {
		fmt.Fprintln(os.Stderr, tui.Error(err.Error()))
		os.Exit(1)
	}
}

type runOptions struct {
	port                       int
	host                       string
	noBrowser, webUI           bool
	interactive                bool
	log, tray, ssh, skipUpdate bool
	sshPort                    int
	setPassword, resetPassword string
}

func run(options runOptions) error {
	port := options.port
	addr := os.Getenv("G9ROUTER_ADDR")
	if options.host != "" || port != 0 {
		host := options.host
		if host == "" {
			host = "0.0.0.0"
		}
		if port == 0 {
			port = 20128
		}
		addr = host + ":" + strconv.Itoa(port)
	}
	if addr == "" {
		addr = ":20128"
	}
	app := server.New(server.Options{Addr: addr, Upstream: os.Getenv("G9ROUTER_UPSTREAM"), APIKey: os.Getenv("G9ROUTER_API_KEY"), WebUI: options.webUI})
	if options.setPassword != "" || options.resetPassword != "" {
		if options.setPassword != "" && options.resetPassword != "" {
			return fmt.Errorf("use only one of --set-password or --reset-password")
		}
		if options.setPassword != "" {
			if app.PasswordConfigured() {
				return fmt.Errorf("password already configured; use --reset-password")
			}
			return app.SetPassword(options.setPassword)
		}
		if os.Getenv("G9ROUTER_PASSWORD") != "" {
			return fmt.Errorf("G9ROUTER_PASSWORD is set; change the environment variable instead")
		}
		if !app.ValidatePassword(options.resetPassword) {
			return fmt.Errorf("invalid current password")
		}
		password, err := tui.ResetPasswordForm(os.Stdin, os.Stdout)
		if err != nil {
			return err
		}
		return app.SetPassword(password)
	}
	if options.ssh && !app.PasswordConfigured() {
		return fmt.Errorf("--ssh requires a configured G9Router password")
	}
	if options.ssh && (options.sshPort < 1 || options.sshPort > 65535) {
		return fmt.Errorf("invalid --ssh-port: %d", options.sshPort)
	}
	ports := []int{portNumber(addr)}
	if options.ssh {
		ports = append(ports, options.sshPort)
	}
	if err := instance.ReleaseOwnedPorts(ports...); err != nil {
		return err
	}
	releaseInstance, err := instance.Acquire(ports...)
	if err != nil {
		return err
	}
	defer releaseInstance()
	var sshListener net.Listener
	var sshServe func(net.Listener) error
	if options.ssh {
		sshAddr := net.JoinHostPort("0.0.0.0", strconv.Itoa(options.sshPort))
		var err error
		sshListener, err = net.Listen("tcp", sshAddr)
		if err != nil {
			return fmt.Errorf("cannot listen on SSH %s: %w", sshAddr, err)
		}
		sshApp, err := sshserver.New(app, sshAddr, "http://127.0.0.1:"+portFromAddr(addr))
		if err != nil {
			_ = sshListener.Close()
			return err
		}
		sshServe = sshApp.Serve
	}
	appHandler := auth.Middleware(app.Handler(), os.Getenv("G9ROUTER_ADMIN_KEY"))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		if sshListener != nil {
			_ = sshListener.Close()
		}
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	fmt.Fprintln(os.Stderr, tui.Info("g9router listening on "+addr))
	errors := make(chan error, 2)
	go func() { errors <- http.Serve(listener, appHandler) }()
	if options.ssh {
		sshAddr := net.JoinHostPort("0.0.0.0", strconv.Itoa(options.sshPort))
		go func() {
			if err := sshServe(sshListener); err != nil {
				fmt.Fprintln(os.Stderr, tui.Error("SSH server stopped: "+err.Error()))
				_ = listener.Close()
				errors <- fmt.Errorf("SSH server stopped: %w", err)
			}
		}()
		fmt.Fprintln(os.Stderr, tui.Info("g9router SSH listening on "+sshAddr))
	}
	ready := true
	if ready && options.tray {
		executable, _ := os.Executable()
		if err := tray.Run(portNumber(addr), executable, func() { os.Exit(0) }); err != nil {
			fmt.Fprintln(os.Stderr, tui.Error("tray unavailable: "+err.Error()))
		}
		return <-errors
	}
	if ready && !options.ssh && (tui.IsTerminal(os.Stdin) || options.interactive) {
		if err := tui.Run(tui.PortURL("127.0.0.1", portNumber(addr)), os.Stdin, os.Stdout); err != nil {
			return fmt.Errorf("interactive CLI: %w", err)
		}
		return nil
	}
	if ready && options.webUI && !options.noBrowser {
		openBrowser("http://localhost:" + portFromAddr(addr))
	}
	return <-errors
}

func openBrowser(target string) {
	var command string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
	case "windows":
		command = "rundll32"
		target = "url.dll,FileProtocolHandler " + target
	default:
		command = "xdg-open"
	}
	if err := exec.Command(command, target).Start(); err != nil {
		fmt.Fprintln(os.Stderr, tui.Info("dashboard available at "+target))
	}
}

func portFromAddr(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return "20128"
}

func portNumber(addr string) int {
	value, err := strconv.Atoi(portFromAddr(addr))
	if err != nil {
		return 20128
	}
	return value
}
