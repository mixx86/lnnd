package peer

import (
	"crypto/ed25519"
	"lnn/internal/identity"
	"net"

	"github.com/google/uuid"
)

type PeerConn struct {
	ID     uuid.UUID
	PubKey ed25519.PublicKey
	Conn   net.Conn
}

type Service struct {
	identity *identity.IdentityService
	peers    *PeerManager
}
