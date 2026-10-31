package network

import (
	"lnn/internal/identity"
	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/sched"
)

type Server struct {
	Transport Transport

	Sessions  *peer.SessionManager
	Scheduler *sched.Service
	Identity  *identity.Service
}

func NewServer(t Transport, sm *peer.SessionManager, sched *sched.Service, i *identity.Service) *Server {
	return &Server{
		Transport: t,

		Sessions:  sm,
		Scheduler: sched,
		Identity:  i,
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

		go HandleConn(
			conn,
			s.Transport,
			s.Sessions,
			s.Scheduler,
			s.Identity,
		)
	}
}
