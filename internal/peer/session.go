package peer

import (
	"crypto/ed25519"
	"lnn/internal/protocol"
	"net"

	"github.com/google/uuid"
)

func NewSelfNode() *SelfNode {
	return &SelfNode{
		Pending: make(map[uint32]chan protocol.Payload),
	}
}

func NewRemoteNode(id uuid.UUID, pubKey ed25519.PublicKey) *RemoteNode {
	return &RemoteNode{
		ID:     id,
		PubKey: pubKey,
	}
}

func NewSession(
	id uuid.UUID,
	pubKey ed25519.PublicKey,
	conn net.Conn,
) *Session {
	return &Session{
		Remote: NewRemoteNode(id, pubKey),
		Self:   NewSelfNode(),
		Conn:   conn,
		State:  StateNew,
	}
}
