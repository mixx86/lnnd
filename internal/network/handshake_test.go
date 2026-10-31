package network

import (
	"net"
	"testing"

	"lnn/internal/identity"
	"lnn/internal/peer"
)

func newIdentity(t *testing.T) *identity.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	id, err := identity.NewService()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestHandshakeOverPipe(t *testing.T) {
	client := newIdentity(t)
	server := newIdentity(t)

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	errc := make(chan error, 1)
	go func() {
		_, err := InboundHandshake(b, server, peer.NewSessionManager())
		errc <- err
	}()

	s, err := OutboundHandshake(a, client, peer.NewSessionManager())
	if err != nil {
		t.Fatalf("outbound: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("inbound: %v", err)
	}
	if s.Remote.ID != server.Credentials().ID {
		t.Fatalf("authenticated %v, want %v",
			s.Remote.ID, server.Credentials().ID)
	}
}
