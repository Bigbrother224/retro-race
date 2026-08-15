// Command relay runs the Retro Race input relay server. It lets two netplay
// players connect from anywhere (no NAT / port-forwarding needed): each player
// connects out to this server and joins a room by code, and the server pipes
// their small input packets between them. It never renders or stores game data.
//
// Usage: retrorace-relay [--addr :9330]
package main

import (
	"flag"
	"log"
	"net"

	"retrorace/internal/relay"
)

func main() {
	addr := flag.String("addr", ":9330", "TCP listen address for the relay")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("relay: listen %s: %v", *addr, err)
	}
	log.Printf("Retro Race relay listening on %s", ln.Addr())
	if err := relay.Serve(ln); err != nil {
		log.Fatalf("relay: %v", err)
	}
}
