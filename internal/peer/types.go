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

type State uint

const (
	  StateVerified State = iota
	  StateClosing
)

func (s *Session) GetState() State     { return State(s.State.Load()) }
func (s *Session) SetState(st State) { s.State.Store(uint32(st)) }

type LinkType uint8

const (
	LinkSym LinkType = iota
	LinkHard
)

type Link struct {
	ID      uint32 // Unique within this peer only, not globally.
	Local   string
	Remote  string
	Type    LinkType
	Created time.Time
}

type RemoteNode struct {
	ID     uuid.UUID
	PubKey ed25519.PublicKey
}

type SelfNode struct {
	PendingMu sync.Mutex
	Pending   map[uint32]chan protocol.Payload
}

type Session struct {
	Remote *RemoteNode
	Self   *SelfNode
	Links  []Link
	Conn   net.Conn
	// Mu guards writes to Conn: the TCP read goroutine and the IPC handler
	// goroutine may write concurrently.
	Mu    sync.Mutex
	State atomic.Uint32
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[uuid.UUID]*Session
}
