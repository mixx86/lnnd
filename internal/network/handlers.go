package network

import (
	"errors"
	"fmt"
	"net"
	"time"

	"lnn/internal/identity"
	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/sched"
)

// 5-second handshake deadline: prevents a slow or malicious peer from holding the goroutine forever.
func HandleConnTCP(conn net.Conn, sm *peer.SessionManager, schedService *sched.Service, idService *identity.Service) {
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
	p, err := InboundHandshake(conn, idService, sm)
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
		err = HandleTCPMessage(conn, msg, p, schedService)
		if err != nil {
			logger.Daemon.Printf("Error managing the message: %v", err)
			return
		}
	}
}

func HandleTCPMessage(conn net.Conn, msg protocol.Frame, s *peer.Session, sc *sched.Service) error {
	switch m := msg.Body.(type) {
	case protocol.Ok:
		return nil
	case protocol.Error:
		return fmt.Errorf("peer %v error %d: %s", s.Remote.ID, m.Code, m.Message)

	case protocol.SymLinkRequest:
		ibrequest, ok := msg.Body.(protocol.InboundRequest)
		if !ok {
			fmt.Errorf("error parsing the inbound request.\n")
		}
		go func() {
			err := sc.SubmitRequest(s.Remote, ibrequest, msg.Header.Tag)

			if err != nil {
				if errors.Is(err, sched.ErrRequestsQueueFull) {
					SendTCP(s, protocol.Frame{
						Body: protocol.Error{
							Code:    protocol.ErrOverloaded,
							Message: protocol.MsgErrOverloaded,
						},
					}.WithTag(msg.Header.Tag))
				}
				SendTCP(s, protocol.Frame{
					Body: protocol.Error{
						Code:    protocol.ErrInternal,
						Message: protocol.MsgErrInternal,
					},
				}.WithTag(msg.Header.Tag))
			}
			SendTCP(s, protocol.Frame{
				Body: protocol.Ok{},
			}.WithTag(msg.Header.Tag))
		}()
		return nil

	case protocol.Hello, protocol.Challenge, protocol.ChallengeResp:
		SendTCP(s, protocol.Frame{
			Body: protocol.Error{
				Code:    protocol.ErrInvalidState,
				Message: protocol.MsgErrInvalidState,
			},
		}.WithTag(msg.Header.Tag))
		return fmt.Errorf("handshake message received in state %v from peer %v", s.State, s.Remote.ID)

	default:
		SendTCP(s, protocol.Frame{
			Body: protocol.Error{
				Code:    protocol.ErrInvalidMessage,
				Message: protocol.MsgErrInvalidMessage,
			},
		})
		return fmt.Errorf("unknown message type from peer %v", s.Remote.ID)
	}
}
