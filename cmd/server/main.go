package main

import (
	"flag"
	"log"

	"github.com/dachi-pa/publictunnel/internal/server"
)

func main() {
	domain := flag.String("domain", "server.publictunnel.com", "Main domain for the server API")
	tunnelDomain := flag.String("tunnel-domain", "publictunnel.com", "Base domain for the tunnels")
	port := flag.Int("port", 4000, "Port to run the server on")
	flag.Parse()

	srv := server.NewTunnelServer(*domain, *tunnelDomain, *port)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
