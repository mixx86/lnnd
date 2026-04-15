// Package daemon implements the main LNN daemon orchestration.
package daemon

import (
	"lnn/internal/identity"
	"lnn/internal/ipc"
	"lnn/internal/network"
	"lnn/internal/peer"
	"log"
)

// Daemon represents the main LNN daemon instance.
type Daemon struct {
	Identity *identity.IdentityService
	Peers    *peer.PeerManager
}

// New creates and initializes a new Daemon instance.
func New() (*Daemon, error) {
	identitySvc, err := identity.NewService()
	if err != nil {
		return nil, err
	}

	peerMgr := peer.NewPeerManager()

	return &Daemon{
		Identity: identitySvc,
		Peers:    peerMgr,
	}, nil
}

// Start initializes and starts all daemon services.
// Starts the TCP network listener in a goroutine and blocks on IPC listener (so the main thread doesn't die).
func (d *Daemon) Start() {
	log.Println("Starting lnnd...")

	go network.ListenTCP(d.Identity, d.Peers)
	ipc.ListenIPC()
}
