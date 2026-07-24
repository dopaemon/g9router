package main

import (
	"log"
	"os"

	"g9router/internal/server"
)

func main() {
	addr := os.Getenv("G9ROUTER_ADDR")
	if addr == "" {
		addr = ":20128"
	}
	app := server.New(server.Options{Addr: addr, Upstream: os.Getenv("G9ROUTER_UPSTREAM"), APIKey: os.Getenv("G9ROUTER_API_KEY")})
	log.Fatal(app.Run())
}
