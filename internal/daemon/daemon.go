package daemon

import (
	"fmt"
	"path/filepath"

	app "lnn/internal"
	"lnn/internal/config"
	"lnn/internal/identity"
	"lnn/internal/logger"
	"lnn/internal/outbox"
	"lnn/internal/recovery"
	"lnn/internal/sched"
	"lnn/internal/store"

	"lnn/internal/ipc"
	"lnn/internal/network"
	"lnn/internal/peer"
)

type Daemon struct {
	app.Services
}

func New() (*Daemon, error) {
	identityService, err := identity.NewService()
	if err != nil {
		return nil, err
	}

	sessionMgr := peer.NewSessionManager()

	// A wrong guess only costs adaptive throttling, so the daemon keeps
	// running without disk monitoring instead of refusing to start.
	diskDevice, err := config.DiskDevice()
	if err != nil {
		logger.Daemon.Printf("disk monitoring disabled: %v", err)
	} else {
		logger.Daemon.Printf("monitoring disk load on %s", diskDevice)
	}
	schedService := sched.NewService(diskDevice)

	dataDir, err := config.DataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get data dir: %w", err)
	}

	sent, err := outbox.Open(filepath.Join(dataDir, "outbox"))
	if err != nil {
		return nil, fmt.Errorf("opening outbox: %w", err)
	}
	pending, err := recovery.Open(filepath.Join(dataDir, "requests.recovery"))
	if err != nil {
		return nil, fmt.Errorf("opening recovery list: %w", err)
	}

	return &Daemon{
		Services: app.Services{
			Identity: identityService,
			Session:  sessionMgr,
			Sched:    schedService,
			Outbox:   sent,
			Recovery: pending,
			Writers: &store.Writers{
				RequestsWriter: *store.NewRequestsWriter(
					dataDir,
					pending,
				),
			},
		},
	}, nil
}

func (d *Daemon) Start() error {
	logger.Daemon.Println("Starting lnnd...")

	go d.Sched.Run()
	go d.Writers.Run(d.Sched)

	cfg := config.DefaultNetworkConfig()
	transport, err := network.NewTransport(cfg)
	if err != nil {
		logger.Daemon.Fatalf("error: %v", err)
	}

	server := network.NewServer(
		transport,
		d.Services.Session,
		d.Services.Sched,
		d.Services.Identity,
		d.Services.Outbox,
		d.Services.Recovery,
	)

	go func() {
		if err := server.Listen(); err != nil {
			logger.Daemon.Fatalf("Listener failed to start: %v", err)
		}
	}()
	return ipc.Listen(d.Services)
}
