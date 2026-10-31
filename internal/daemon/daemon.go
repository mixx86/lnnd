package daemon

import (
	"fmt"

	app "lnn/internal"
	"lnn/internal/config"
	"lnn/internal/identity"
	"lnn/internal/logger"
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
	logger.Daemon.Println("Starting lnnd...")


	go d.Sched.Run()
	go d.Writers.Run(d.Sched)

	cfg := config.DefaultNetworkConfig()
	transport, err := network.NewTransport(cfg)
	if err != nil {
		logger.Daemon.Fatalf("error: %v", err)
	}

	server := network.NewServer(transport, d.Services.Session, d.Services.Sched, d.Services.Identity)

	go func() {
		if err := server.Listen(); err != nil {
			logger.Daemon.Fatalf("Listener failed to start: %v", err)
		}
	}()
	return ipc.ListenIPC(d.Services)
}
