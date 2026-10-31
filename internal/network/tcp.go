package network

import (
	"context"
	"lnn/internal/config"
	"time"

	"net"
)

type TCPTransport struct {
	KeepAlive       bool
	KeepAlivePeriod time.Duration
	Port            string
}

func NewTCPTransport(cfg config.TCPConfig) *TCPTransport {
	return &TCPTransport{
		KeepAlive:       cfg.KeepAlive,
		KeepAlivePeriod: cfg.KeepAlivePeriod,
		Port:            cfg.Port,
	}
}

func (t *TCPTransport) Dial(
	ctx context.Context,
	address string,
) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", address)
}

func (t *TCPTransport) Listen() (net.Listener, error) {
	return net.Listen("tcp", ":"+t.Port)
}

func (t *TCPTransport) ConfigureKeepAlive(conn net.Conn) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}

	if err := tcpConn.SetKeepAlive(t.KeepAlive); err != nil {
		return err
	}

	if t.KeepAlive {
		return tcpConn.SetKeepAlivePeriod(t.KeepAlivePeriod)
	}

	return nil
}
