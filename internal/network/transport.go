package network

import (
	"context"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
)

type Transport interface {
	Listen() (net.Listener, error)
	Dial(ctx context.Context, address string) (net.Conn, error)
}

func SendFrame(
	s *peer.Session,
	frame protocol.Frame,
) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	return WriteFrame(s.Conn, frame)
}

/* This function must only be used there's no mutual exclusion mechanism
 * (e.g. Pre-Handshake stage). Shouldn't cause any problem as long
 * as the conn is owned by a single goroutine, but there aren't guarantees, so,
 * please be careful.
 */
func UnsafeSendFrame(conn net.Conn, frame protocol.Frame) error {
	return WriteFrame(conn, frame)
}
