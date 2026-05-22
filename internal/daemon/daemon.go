package daemon

import (
	"fmt"
	"log"

	"lnn/internal/config"
	"lnn/internal/identity"
	"lnn/internal/iocore"
	"lnn/internal/ipc"
	"lnn/internal/network"
	"lnn/internal/peer"
	"lnn/internal/wal"
)

// Daemon represents the main LNN daemon instance.
type Daemon struct {
	Identity *identity.IdentityService
	Peers    *peer.PeerManager
	IO       *iocore.Manager
	WAL      *wal.Writer
}

// New creates and initializes a new Daemon instance.
func New() (*Daemon, error) {
	identitySvc, err := identity.NewService()
	if err != nil {
		return nil, err
	}

	peerMgr := peer.NewPeerManager()
	ioMgr := iocore.New()

	dataDir, err := config.DataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get data dir: %w", err)
	}
	walWriter := wal.NewWriter(dataDir, ioMgr)

	return &Daemon{
		Identity: identitySvc,
		Peers:    peerMgr,
		IO:       ioMgr,
		WAL:      walWriter,
	}, nil
}

// TCP runs in a goroutine; IPC blocks main — that's what keeps the process alive.
func (d *Daemon) Start() error {
	log.Println("Starting lnnd...")

	go d.WAL.Run()

	go func() {
		// TCP bind failure is fatal: the daemon cannot accept peer connections without it.
		if err := network.ListenTCP(d.Identity, d.Peers, d.IO); err != nil {
			log.Fatalf("TCP listener failed to start: %v", err)
		}
	}()
	return ipc.ListenIPC()
}
