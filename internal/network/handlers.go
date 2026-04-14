package network

import (
	"lnn/internal/identity"
	"lnn/internal/peer"
	"net"
)

func HandleConnTCP(conn net.Conn, i *identity.IdentityService, p *peer.PeerManager) {
	defer conn.Close()
	err := OutBoundHandshake(conn, i, p)
	if err != nil {
		return
	}
}
