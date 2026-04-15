// Package network handles TCP connections and protocol handshakes.
package network

import (
	"lnn/internal/identity"
	"lnn/internal/peer"
	"net"
	"time"
)

// HandleConnTCP manages an incoming TCP connection, performing handshake with timeout.
func HandleConnTCP(conn net.Conn, i *identity.IdentityService, p *peer.PeerManager) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	err := InBoundHandshake(conn, i, p)
	if err != nil {
		return
	}
	conn.SetDeadline(time.Time{})

}
