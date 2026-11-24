package network

import (
	"errors"
	"fmt"
	"net"
	"time"

	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/sched"
)

func HandleConn(conn net.Conn, srv *Server) {
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Daemon.Printf(
				"Error closing connection %s: %v",
				conn.RemoteAddr(),
				err,
			)
		}
	}()
	if err := srv.prepareConn(conn); err != nil {
		logger.Daemon.Printf(
			"Error preparing connection %s: %v",
			conn.RemoteAddr(),
			err,
		)
		return
	}
	p, err := InboundHandshake(conn, srv.Identity, srv.Sessions)
	if err != nil {
		logger.Daemon.Printf("Error during the handshake: %v", err)
		return
	}
	defer shutdownSession(srv, p)
	if err = conn.SetDeadline(time.Time{}); err != nil {
		logger.Daemon.Printf("Error clearing deadline in %s", conn.RemoteAddr())
		return
	}

	serveSession(conn, p, srv)
}

func shutdownSession(srv *Server,s *peer.Session) {
	srv.Sessions.Unregister(s)
	s.SetState(peer.StateClosing)
	s.Self.AbortPendingMessages()
}

// WARNING: This function assumes that handshake has been
// succesfully completed
func serveSession(conn net.Conn, s *peer.Session, srv *Server) {
	// Sent concurrently so the read loop is already draining replies; both
	// sides writing without reading could otherwise fill the socket.
	go requestRecoveries(srv, s)

	for {
		msg, err := ReadFrame(conn)
		if err != nil {
			if errors.Is(err, protocol.MsgErrInvalidMessage) {
				logger.Protocol.Printf("Invalid message: %v", err)
				SendFrame(s, protocol.Frame{
					Body: protocol.Error{
						Code:    protocol.ErrInvalidMessage,
						Message: protocol.MsgErrInvalidMessage.Error(),
					},
				}.WithTag(msg.Header.Tag))
				continue
			}
			logger.Daemon.Printf("Error receiving packets: %v", err)
			return
		}
		err = HandleTCPMessage(conn, msg, s, srv)
		if err != nil {
			logger.Daemon.Printf("Error managing the message: %v", err)
		}
	}
}

func HandleTCPMessage(
	conn net.Conn,
	msg protocol.Frame,
	s *peer.Session,
	srv *Server,
) error {
	tag := msg.Header.Tag

	switch m := msg.Body.(type) {
	case protocol.OK:
		srv.Outbox.Ack(s.Remote.ID, tag)
		s.Self.ResolveMessage(tag, m)
		return nil

	case protocol.Error:
		if tag == 0 {
			return fmt.Errorf(
				"peer %v error %d: %s",
				s.Remote.ID,
				m.Code,
				m.Message,
			)
		}
		srv.Outbox.Ack(s.Remote.ID, tag)
		if m.Code == protocol.ErrUnknownTag {
			dropRecovery(srv, s, tag)
		}
		s.Self.ResolveMessage(tag, m)
		logger.Daemon.Printf(
			"peer %v error %d for tag %d: %s",
			s.Remote.ID,
			m.Code,
			tag,
			m.Message,
		)
		return nil

	case protocol.RecoverRequest:
		frame, ok := srv.Outbox.Get(s.Remote.ID, tag)
		if !ok {
			frame = protocol.Frame{
				Body: protocol.Error{
					Code:    protocol.ErrUnknownTag,
					Message: protocol.MsgErrUnknownTag.Error(),
				},
			}.WithTag(tag)
		}
		return SendFrame(s, frame)

	case protocol.SymlinkRequest:
		go submitRequest(srv, s, m, tag)
		return nil

	case protocol.Hello, protocol.Challenge, protocol.ChallengeResp:
		SendFrame(s, protocol.Frame{
			Body: protocol.Error{
				Code:    protocol.ErrInvalidState,
				Message: protocol.MsgErrInvalidState.Error(),
			},
		}.WithTag(msg.Header.Tag))
		return fmt.Errorf(
			"handshake message received in state %v from peer %v",
			s.State,
			s.Remote.ID,
		)

	default:
		SendFrame(s, protocol.Frame{
			Body: protocol.Error{
				Code:    protocol.ErrInvalidMessage,
				Message: protocol.MsgErrInvalidMessage.Error(),
			},
		}.WithTag(msg.Header.Tag))
		return fmt.Errorf("unknown message type from peer %v", s.Remote.ID)
	}
}

func submitRequest(
	srv *Server,
	s *peer.Session,
	req protocol.InboundRequest,
	tag uint32,
) {
	err := srv.Scheduler.SubmitRequest(s.Remote, req, tag)
	if err != nil {
		reply := protocol.Error{
			Code:    protocol.ErrInternal,
			Message: protocol.MsgErrInternal.Error(),
		}
		if errors.Is(err, sched.ErrRequestsQueueFull) ||
			errors.Is(err, sched.ErrOverloaded) {
			reply = protocol.Error{
				Code:    protocol.ErrOverloaded,
				Message: protocol.MsgErrOverloaded.Error(),
			}
		}
		_ = SendFrame(s, protocol.Frame{Body: reply}.WithTag(tag))
		return
	}

	// A resend after RECOVER_REQUEST is just a request carrying an owed
	// tag: once it is durable, nothing is owed any more.
	if e, ok, err := srv.Recovery.Resolve(s.Remote.ID, tag); err != nil {
		logger.Daemon.Printf("resolving recovery: %v", err)
	} else if ok {
		logger.Daemon.Printf(
			"recovered request %d from peer %v",
			e.LCLReqID,
			s.Remote.ID,
		)
	}

	_ = SendFrame(s, protocol.Frame{Body: protocol.OK{}}.WithTag(tag))
}

// requestRecoveries asks the peer for every request lost in a crash.
// Entries stay pending until the resend becomes durable or the peer answers
// ErrUnknownTag, so a dropped connection simply retries on the next one.
func requestRecoveries(srv *Server, s *peer.Session) {
	for _, e := range srv.Recovery.ForPeer(s.Remote.ID) {
		err := SendFrame(s, protocol.Frame{
			Body: protocol.RecoverRequest{},
		}.WithTag(e.Tag))
		if err != nil {
			logger.Daemon.Printf(
				"requesting recovery of request %d from peer %v: %v",
				e.LCLReqID,
				s.Remote.ID,
				err,
			)
			return
		}
	}
}

func dropRecovery(srv *Server, s *peer.Session, tag uint32) {
	e, ok, err := srv.Recovery.Resolve(s.Remote.ID, tag)
	if err != nil {
		logger.Daemon.Printf("resolving recovery: %v", err)
		return
	}
	if ok {
		logger.Daemon.Printf(
			"peer %v no longer has request %d (tag %d); dropping it",
			s.Remote.ID,
			e.LCLReqID,
			tag,
		)
	}
}
