package app

import (
	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/sched"
	"lnn/internal/store"
)

type Services struct {
	Identity *identity.Service
	Session  *peer.SessionManager
	Sched    *sched.Service
	Writers  *store.Writers
}
