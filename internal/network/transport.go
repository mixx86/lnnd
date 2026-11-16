package network

import (
	"context"
	"errors"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
)

var ErrSessionClosed = errors.New("session closed")

type Transport interface {
	Listen() (net.Listener, error)
	Dial(ctx context.Context, address string) (net.Conn, error)
}

func SendFrame(
	s *peer.Session,
	frame protocol.Frame,
) error {
	if s.GetState() == peer.StateClosing {
		  return ErrSessionClosed
  	}
	s.Mu.Lock()
	defer s.Mu.Unlock()

	return WriteFrame(s.Conn, frame)
}

// UnsafeSendFrame skips Session.Mu because before the handshake there is no
// session to lock. It is only safe while a single goroutine owns conn.
func UnsafeSendFrame(conn net.Conn, frame protocol.Frame) error {
	return WriteFrame(conn, frame)
}
