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

// Start launches all daemon services and blocks until the IPC listener exits.
// Both listeners block indefinitely, so TCP runs in a background goroutine
// while IPC holds the calling goroutine — this keeps the process alive.
func (d *Daemon) Start() error {
	log.Println("Starting lnnd...")

	go func() {
		// TCP bind failure is fatal: the daemon cannot accept peer connections without it.
		if err := network.ListenTCP(d.Identity, d.Peers); err != nil {
			log.Fatalf("TCP listener failed to start: %v", err)
		}
	}()
	return ipc.ListenIPC()
}
