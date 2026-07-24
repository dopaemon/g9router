package main

import (
	"log"
	"net/http"
	"os"

	"g9router/internal/auth"
	"g9router/internal/server"
)

func main() {
	addr := os.Getenv("G9ROUTER_ADDR")
	if addr == "" {
		addr = ":20128"
	}
	app := server.New(server.Options{Addr: addr, Upstream: os.Getenv("G9ROUTER_UPSTREAM"), APIKey: os.Getenv("G9ROUTER_API_KEY")})
	appHandler := auth.Middleware(app.Handler(), os.Getenv("G9ROUTER_ADMIN_KEY"))
	log.Printf("g9router listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, appHandler))
}
