package peer

import (
	"crypto/ed25519"
	"net"
	"sync"
	"sync/atomic"
	"time"

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

// LinkType distinguishes between symbolic and hard links.
type LinkType uint8

const (
	LinkSym  LinkType = iota
	LinkHard
)

// Link represents an active remote link established with a peer.
type Link struct {
	ID      uint32 // Unique within this peer only, not globally.
	Local   string
	Remote  string
	Type    LinkType
	Created time.Time
}

// Peer represents an authenticated remote peer and its associated connection.
type Peer struct {
	ID     uuid.UUID
	PubKey ed25519.PublicKey
	Conn   net.Conn
	State  State
	Links  []Link
	// Mu guards writes to Conn. Exported so network.SendTCP can lock it directly;
	// both the TCP read goroutine and the IPC handler goroutine may write concurrently.
	Mu        sync.Mutex
	nextReqID atomic.Uint32
}

// NextReqID returns a new request ID scoped to this peer, starting from 1.
func (p *Peer) NextReqID() uint32 {
	return p.nextReqID.Add(1)
}

// PeerManager maintains a thread-safe registry of active peer connections.
type PeerManager struct {
	mu    sync.RWMutex
	peers map[uuid.UUID]*Peer
}
