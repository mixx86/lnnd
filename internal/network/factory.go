package network

import (
	"fmt"
	"lnn/internal/config"
)

func NewTransport(cfg config.NetworkConfig) (Transport, error) {
    switch cfg.Transport {
    case "tcp":
        return NewTCPTransport(cfg.TCP), nil

//    case "quic":
        //return NewQUICTransport(cfg.QUIC)

    default:
        return nil, fmt.Errorf(
            "unsupported transport: %s",
            cfg.Transport,
        )
    }
}
