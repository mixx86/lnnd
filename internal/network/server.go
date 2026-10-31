package network

import (
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
