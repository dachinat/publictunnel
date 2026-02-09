package main

import (
	"flag"
	"log"

	"github.com/dachinat/publictunnel/internal/client"
)

func main() {
	localPort := flag.Int("port", 8080, "Local port to forward to")
	subdomain := flag.String("subdomain", "", "Request a specific subdomain")
	flag.Parse()

	serverURL := "https://server.publictunnel.com"
	c := client.NewTunnelClient(serverURL, *localPort, *subdomain)
	if err := c.Start(); err != nil {
		log.Fatalf("Client failed: %v", err)
	}
}
