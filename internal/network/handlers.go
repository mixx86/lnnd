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

// HandleConnTCP manages an incoming TCP connection, performing handshake with timeout.
func HandleConnTCP(conn net.Conn, i *identity.IdentityService, pm *peer.PeerManager) {
	defer conn.Close()
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	} else {
		logger.Daemon.Printf("Couldn't activate KeepAlive in %s", conn.RemoteAddr())
	}

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	p, err := InBoundHandshake(conn, i, pm)
	if err != nil {
		logger.Daemon.Printf("Error during the handshake: %v", err)
		return
	}
	conn.SetDeadline(time.Time{})

	for {
		msg, err := ReceiveTCP(conn)
		if err != nil {
			logger.Daemon.Printf("Error receving TCP packets: %v", err)
			return
		}
		err = HandleTCPMessage(conn, msg, &p)
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
	case protocol.HelloMessage, protocol.ChallengeMessage, protocol.ChallengeRespMessage:
		SendTCP(conn, protocol.ErrorMessage{
			Code:    protocol.ErrInvalidState,
			Message: protocol.MsgErrInvalidState,
		})
		return fmt.Errorf("handshake message received in state %v from peer %v", p.State, p.ID)

	case protocol.ErrorMessage:
		return fmt.Errorf("peer %v error %d: %s", p.ID, m.Code, m.Message)

	default:
		SendTCP(conn, protocol.ErrorMessage{
			Code:    protocol.ErrInvalidMessage,
			Message: protocol.MsgErrInvalidMessage,
		})
		return fmt.Errorf("unknown message type from peer %v", p.ID)
	}
}
