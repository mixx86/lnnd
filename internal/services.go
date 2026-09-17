package app

import (
	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/sched"
)

type Services struct {
	Identity *identity.Service
	Session  *peer.SessionManager
	Sched    *sched.Service
}
