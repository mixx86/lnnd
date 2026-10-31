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
	return d.DialContext(ctx, "tcp", t.withPort(address))
}

// withPort lets callers pass a bare IP, since every peer listens on the
// same configured port. JoinHostPort instead of concatenation because IPv6
// needs brackets ("::1" -> "[::1]:5771"); an address that already has a
// port is kept as is.
func (t *TCPTransport) withPort(address string) string {
	if _, _, err := net.SplitHostPort(address); err == nil {
		return address
	}
	return net.JoinHostPort(address, t.Port)
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
