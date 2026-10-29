package daemon

import (
	"fmt"
	"log"

	app "lnn/internal"
	"lnn/internal/config"
	"lnn/internal/identity"
	"lnn/internal/sched"
	"lnn/internal/store"

	"lnn/internal/ipc"
	"lnn/internal/network"
	"lnn/internal/peer"
)

// Daemon represents the main LNN daemon instance.
type Daemon struct {
	app.Services
}

// New creates and initializes a new Daemon instance.
func New() (*Daemon, error) {
	identityService, err := identity.NewService()
	if err != nil {
		return nil, err
	}

	sessionMgr := peer.NewSessionManager()
	schedService := sched.NewService()

	dataDir, err := config.DataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get data dir: %w", err)
	}

	return &Daemon{
		Services: app.Services{
			Identity: identityService,
			Session:  sessionMgr,
			Sched:    schedService,
			Writers: &store.Writers{
				*store.NewRequestsWriter(dataDir, sessionMgr),
			},
		},
	}, nil
}

func (d *Daemon) Start() error {
	log.Println("Starting lnnd...")

	go d.Sched.Run()
	go d.Writers.Run(d.Sched)

	go func() {
		if err := network.ListenTCP(d.Session, d.Sched, d.Identity); err != nil {
			log.Fatalf("TCP listener failed to start: %v", err)
		}
	}()
	return ipc.ListenIPC(d.Services)
}
