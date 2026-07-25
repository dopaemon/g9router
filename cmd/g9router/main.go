package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"g9router/internal/auth"
	"g9router/internal/cli/tray"
	"g9router/internal/cli/tui"
	"g9router/internal/cli/xai"
	"g9router/internal/server"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "xai" && os.Args[2] == "video" {
		if tui.IsTerminal(os.Stdin) {
			os.Exit(xai.RunTTY(context.Background(), os.Stdout))
		}
		os.Exit(xai.Run(context.Background(), os.Args[3:], os.Stdout, os.Stderr))
	}
	port := flag.Int("port", 0, "port to run the server")
	flag.IntVar(port, "p", 0, "port to run the server")
	host := flag.String("host", "", "host to bind")
	flag.StringVar(host, "H", "", "host to bind")
	noBrowser := flag.Bool("no-browser", false, "do not open the dashboard")
	flag.BoolVar(noBrowser, "n", false, "do not open the dashboard")
	flag.Bool("log", false, "show server logs")
	trayMode := flag.Bool("tray", false, "run in system tray mode")
	flag.Bool("skip-update", false, "skip update check")
	version := flag.Bool("version", false, "show version")
	flag.Parse()
	if *version {
		fmt.Fprintln(os.Stdout, tui.Success("9Router v0.5.40"))
		return
	}
	addr := os.Getenv("G9ROUTER_ADDR")
	if *host != "" || *port != 0 {
		if *host == "" {
			*host = "0.0.0.0"
		}
		if *port == 0 {
			*port = 20128
		}
		addr = *host + ":" + strconv.Itoa(*port)
	}
	if addr == "" {
		addr = ":20128"
	}
	app := server.New(server.Options{Addr: addr, Upstream: os.Getenv("G9ROUTER_UPSTREAM"), APIKey: os.Getenv("G9ROUTER_API_KEY")})
	appHandler := auth.Middleware(app.Handler(), os.Getenv("G9ROUTER_ADMIN_KEY"))
	fmt.Fprintln(os.Stderr, tui.Info("g9router listening on "+addr))
	errors := make(chan error, 1)
	go func() { errors <- http.ListenAndServe(addr, appHandler) }()
	ready := waitReady(addr, 15*time.Second)
	if ready && *trayMode {
		executable, _ := os.Executable()
		if err := tray.Run(portNumber(addr), executable, func() { os.Exit(0) }); err != nil {
			log.Printf("tray unavailable: %v", err)
		}
		log.Fatal(<-errors)
	}
	if ready && tui.IsTerminal(os.Stdin) {
		_ = tui.Run(tui.PortURL("127.0.0.1", portNumber(addr)), os.Stdin, os.Stdout)
		return
	}
	if ready && !*noBrowser {
		openBrowser("http://localhost:" + portFromAddr(addr))
	}
	log.Fatal(<-errors)
}

func waitReady(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
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
