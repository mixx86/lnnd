package network

import (
	"fmt"
	"lnn/internal/config"
)

func NewTransport(cfg config.NetworkConfig) (Transport, error) {
	switch cfg.Transport {
	case "tcp":
		return NewTCPTransport(cfg.TCP), nil

	default:
		return nil, fmt.Errorf(
			"unsupported transport: %s",
			cfg.Transport,
		)
	}
}
