package peer

import (
	"crypto/ed25519"
	"net"
	"sync"
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
	LinkSym  LinkType = iota // Symbolic link — points to a path, survives file replacement.
	LinkHard                 // Hard link — shares the inode, breaks if the original is deleted.
)

// Link represents an active remote link established with a peer.
type Link struct {
	ID      uint32 // Unique within this peer; auto-incremented, not globally unique.
	Local   string // Path on this machine where the link was created.
	Remote  string // Path on the peer's machine that the link points to.
	Type    LinkType
	Created time.Time
}

// Peer represents an authenticated remote peer and its associated connection.
type Peer struct {
	ID     uuid.UUID
	PubKey ed25519.PublicKey
	Conn   net.Conn
	State  State
	Links  []Link // Active links established with this peer. Lifecycle mirrors the peer's.
	// Mu guards writes to Conn. Exported so network.SendTCP can lock it directly;
	// both the TCP read goroutine and the IPC handler goroutine may write concurrently.
	Mu sync.Mutex
}

// PeerManager maintains a thread-safe registry of active peer connections.
type PeerManager struct {
	mu    sync.RWMutex
	peers map[uuid.UUID]*Peer
}
