package daemon

import (
	"lnn/internal/identity"
	"lnn/internal/ipc"
	"lnn/internal/network"
	"lnn/internal/peer"
	"log"
)

type Daemon struct {
	Identity *identity.IdentityService
	Peers    *peer.PeerManager
}

func New() (*Daemon, error) {
	idSvc, err := identity.NewService()
	if err != nil {
		return nil, err
	}

	peerMgr := peer.NewPeerManager()

	return &Daemon{
		Identity: idSvc,
		Peers:    peerMgr,
	}, nil
}

func (d *Daemon) Start() {
	log.Println("Starting lnnd...")

	go network.ListenTCP(d.Identity, d.Peers)
	ipc.ListenIPC()
}
