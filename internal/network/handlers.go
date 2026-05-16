// Package network handles TCP connections and protocol handshakes.
package network

import (
	"fmt"
	"lnn/internal/identity"
	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
	"time"
)

// HandleConnTCP manages a single incoming TCP connection from handshake to close.
// Enforces a 5-second deadline on the handshake phase to prevent a slow or malicious
// client from holding the goroutine open indefinitely before authenticating.
func HandleConnTCP(conn net.Conn, i *identity.IdentityService, pm *peer.PeerManager) {
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Daemon.Printf("Error closing connection %s: %v", conn.RemoteAddr(), err)
		}
	}()
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// Keepalive lets the OS detect dead connections without application-level heartbeats.
		// Failure is non-fatal: the connection still works, we just lose dead-peer detection.
		if err := tcpConn.SetKeepAlive(true); err != nil {
			logger.Daemon.Printf("Couldn't activate KeepAlive in %s: %v", conn.RemoteAddr(), err)
		}
		if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
			logger.Daemon.Printf("Couldn't set KeepAlive period in %s: %v", conn.RemoteAddr(), err)
		}
	} else {
		logger.Daemon.Printf("Couldn't activate KeepAlive in %s", conn.RemoteAddr())
	}

	if conn.SetDeadline(time.Now().Add(5*time.Second)) != nil {
		logger.Daemon.Printf("Couldn't set deadline in %s", conn.RemoteAddr())
		return
	}
	p, err := InBoundHandshake(conn, i, pm)
	if err != nil {
		logger.Daemon.Printf("Error during the handshake: %v", err)
		return
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		logger.Daemon.Printf("Error clearing deadline in %s", conn.RemoteAddr())
		return
	}

	for {
		msg, err := ReceiveTCP(conn)
		if err != nil {
			logger.Daemon.Printf("Error receving TCP packets: %v", err)
			return
		}
		err = HandleTCPMessage(conn, msg, p)
		if err != nil {
			logger.Daemon.Printf("Error managing the message: %v", err)
			return
		}
	}
}

// HandleTCPMessage processes application-layer messages for peers that have
// successfully completed the handshake (StateVerified).
// It assumes the peer state has already been validated by the routing layer
// and does not handle handshake or pre-verification messages.
func HandleTCPMessage(conn net.Conn, msg protocol.Message, p *peer.Peer) error {
	switch m := msg.(type) {
	case protocol.ErrorMessage:
		return fmt.Errorf("peer %v error %d: %s", p.ID, m.Code, m.Message)

	case protocol.HelloMessage, protocol.ChallengeMessage, protocol.ChallengeRespMessage:
		SendTCP(p, protocol.ErrorMessage{
			Code:    protocol.ErrInvalidState,
			Message: protocol.MsgErrInvalidState,
		})
		return fmt.Errorf("handshake message received in state %v from peer %v", p.State, p.ID)

	default:
		SendTCP(p, protocol.ErrorMessage{
			Code:    protocol.ErrInvalidMessage,
			Message: protocol.MsgErrInvalidMessage,
		})
		return fmt.Errorf("unknown message type from peer %v", p.ID)
	}
}
