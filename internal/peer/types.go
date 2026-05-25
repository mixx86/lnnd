package peer

import (
	"crypto/ed25519"
	"lnn/internal/protocol"
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

// Peer holds the identity of an authenticated remote peer.
type RemoteNode struct {
  ID     uuid.UUID
  PubKey ed25519.PublicKey
}

// SelfNode holds the local runtime data this node maintains for a peer connection.
type SelfNode struct {
  Tag   atomic.Uint32
	PendingMu  sync.Mutex
  Pending map[uint32]chan protocol.Message
}

type Session struct {
  Remote *RemoteNode
  Self   *SelfNode
	Links  []Link
	Conn   net.Conn
	// Mu guards writes to Conn; both the TCP read goroutine and the IPC handler goroutine may write concurrently.
	Mu     sync.Mutex
	State   State
}

// SessionManager maintains a thread-safe registry of active sessions.
type SessionManager struct {
	mu    sync.RWMutex
	sessions map[uuid.UUID]*Session
}
