package peer

import (
	"crypto/ed25519"
	"net"
	"sync"

	"github.com/google/uuid"
)

type State int

const (
	StateNew       State = iota // Recently opened connection
	StateHandshake              // Waiting for Hello/Challenge
	StateVerified               // Authenticated, ready for commands
	StateClosing                // Closing resources
)

// Peer represents an authenticated connection to a remote peer.
type Peer struct {
	ID     uuid.UUID
	PubKey ed25519.PublicKey
	Conn   net.Conn
	State  State
}

// PeerManager maintains a thread-safe registry of active peer connections.
type PeerManager struct {
	mu    sync.RWMutex
	peers map[uuid.UUID]*Peer
}
