package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"g9router/internal/auth"
	"g9router/internal/cli/xai"
	"g9router/internal/server"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "xai" && os.Args[2] == "video" {
		os.Exit(xai.Run(context.Background(), os.Args[3:], os.Stdout, os.Stderr))
	}
	port := flag.Int("port", 0, "port to run the server")
	host := flag.String("host", "", "host to bind")
	noBrowser := flag.Bool("no-browser", false, "do not open the dashboard")
	flag.BoolVar(noBrowser, "n", false, "do not open the dashboard")
	version := flag.Bool("version", false, "show version")
	flag.Parse()
	if *version {
		println("0.5.40")
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
	log.Printf("g9router listening on %s", addr)
	if !*noBrowser {
		openBrowser("http://localhost:" + portFromAddr(addr))
	}
	log.Fatal(http.ListenAndServe(addr, appHandler))
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
		log.Printf("dashboard available at %s", target)
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
