package network

import (
	"fmt"
	"net"
	"time"

	"lnn/internal/identity"
	"lnn/internal/iocore"
	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/protocol"
)

// 5-second handshake deadline: prevents a slow or malicious peer from holding the goroutine forever.
func HandleConnTCP(conn net.Conn, i *identity.IdentityService, sm *peer.SessionManager, ioc *iocore.Manager) {
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Daemon.Printf("Error closing connection %s: %v", conn.RemoteAddr(), err)
		}
	}()
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// Failure is non-fatal: we just lose OS-level dead-peer detection.
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
	p, err := InBoundHandshake(conn, i, sm)
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
		err = HandleTCPMessage(conn, msg, p, ioc)
		if err != nil {
			logger.Daemon.Printf("Error managing the message: %v", err)
			return
		}
	}
}

// HandleTCPMessage processes application-layer messages for peers in StateVerified.
func HandleTCPMessage(conn net.Conn, msg protocol.Message, s *peer.Session, ioc *iocore.Manager) error {
	switch m := msg.(type) {
	case protocol.ErrorMessage:
		return fmt.Errorf("peer %v error %d: %s", s.Remote.ID, m.Code, m.Message)

	case protocol.SymLinkRequest:
		pr, ok := ioc.TryEnqueue(s.Remote, m)
		if !ok {
			if ioc.Pressure() == 0 {
				// WAL is the bottleneck — genuine overload, fail immediately.
				SendTCP(s, protocol.ErrorMessage{
					Code:    protocol.ErrOverloaded,
					Message: protocol.MsgErrOverloaded,
				})
				return fmt.Errorf("WAL overloaded, rejected request from peer %v", s.Remote.ID)
			}

			pr, ok = ioc.Enqueue(s.Remote, m, 5*time.Second)
			if !ok {
				SendTCP(s, protocol.ErrorMessage{
					Code:    protocol.ErrOverloaded,
					Message: protocol.MsgErrOverloaded,
				})
				return fmt.Errorf("backpressure timeout, rejected request from peer %v", s.Remote.ID)
			}
		}
		go func() {
			<-pr.Written
			// sendTCP ok
		}()
		return nil

	case protocol.HelloMessage, protocol.ChallengeMessage, protocol.ChallengeRespMessage:
		SendTCP(s, protocol.ErrorMessage{
			Code:    protocol.ErrInvalidState,
			Message: protocol.MsgErrInvalidState,
		})
		return fmt.Errorf("handshake message received in state %v from peer %v", s.State, s.Remote.ID)

	default:
		SendTCP(s, protocol.ErrorMessage{
			Code:    protocol.ErrInvalidMessage,
			Message: protocol.MsgErrInvalidMessage,
		})
		return fmt.Errorf("unknown message type from peer %v", s.Remote.ID)
	}
}
