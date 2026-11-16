package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"lnn/internal/identity"
	"lnn/internal/logger"
	"lnn/internal/outbox"
	"lnn/internal/peer"
	"lnn/internal/recovery"
	"lnn/internal/sched"
)

type Server struct {
	Transport Transport

	Sessions  *peer.SessionManager
	Scheduler *sched.Service
	Identity  *identity.Service
	Outbox    *outbox.Outbox
	Recovery  *recovery.Pending
}

func NewServer(
	t Transport,
	sm *peer.SessionManager,
	sched *sched.Service,
	i *identity.Service,
	ob *outbox.Outbox,
	rp *recovery.Pending,
) *Server {
	return &Server{
		Transport: t,

		Sessions:  sm,
		Scheduler: sched,
		Identity:  i,
		Outbox:    ob,
		Recovery:  rp,
	}
}

func (s *Server) Listen() error {
	ln, err := s.Transport.Listen()
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Daemon.Printf("Error accepting connection: %v", err)
			continue
		}

		go HandleConn(conn, s)
	}
}

func (s *Server) Connect(
	ctx context.Context,
	address string,
) (*peer.Session, error) {
	conn, err := s.Transport.Dial(ctx, address)
	if err != nil {
		return nil, err
	}
	if err := s.prepareConn(conn); err != nil {
		conn.Close()
		return nil, err
	}
	p, err := OutboundHandshake(conn, s.Identity, s.Sessions)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		shutdownSession(s, p)
		conn.Close()
		return nil, err
	}

	go func() {
		serveSession(conn, p, s)
		shutdownSession(s, p)
		if err := conn.Close(); err != nil {
			logger.Daemon.Printf(
				"Error closing connection %s: %v",
				conn.RemoteAddr(),
				err,
			)
		}
	}()
	return p, nil
}

func (s *Server) prepareConn(conn net.Conn) error {
	if keepAlive, ok := s.Transport.(interface {
		ConfigureKeepAlive(net.Conn) error
	}); ok {
		if err := keepAlive.ConfigureKeepAlive(conn); err != nil {
			return fmt.Errorf("configure keepalive: %w", err)
		}
	}
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set handshake deadline: %w", err)
	}
	return nil
}
