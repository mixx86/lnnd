package peer

import (
	"crypto/ed25519"
	"net"
	"sync"

	"github.com/google/uuid"
)

// State represents the lifecycle stage of a peer connection.
type State int

const (
	StateNew       State = iota // TCP connection accepted, handshake not yet started.
	StateHandshake              // Handshake in progress; only handshake messages are valid.
	StateVerified               // Mutual authentication complete; peer is trusted for application messages.
	StateClosing                // Graceful shutdown initiated; no new messages should be sent.
)

// Peer represents an authenticated remote peer and its associated connection.
type Peer struct {
	ID     uuid.UUID
	PubKey ed25519.PublicKey
	Conn   net.Conn
	State  State
	// Mu guards writes to Conn. Exported so network.SendTCP can lock it directly;
	// both the TCP read goroutine and the IPC handler goroutine may write concurrently.
	Mu sync.Mutex
}

// PeerManager maintains a thread-safe registry of active peer connections.
type PeerManager struct {
	mu    sync.RWMutex
	peers map[uuid.UUID]*Peer
}
