package network

import (
	"net"
	"testing"

	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/protocol"
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

// impostor claims victim's UUID and public key but can only sign with its
// own private key.
type impostor struct {
	victim identity.Credentials
	own    identity.Credentials
}

func newImpostor(t *testing.T) impostor {
	t.Helper()
	return impostor{
		victim: newIdentity(t).Credentials(),
		own:    newIdentity(t).Credentials(),
	}
}

func (i impostor) hello() protocol.Frame {
	return protocol.Frame{Body: protocol.Hello{
		ID:        i.victim.ID,
		PublicKey: i.victim.PubKey,
	}}
}

func (i impostor) answer(c protocol.Challenge) protocol.Frame {
	return protocol.Frame{Body: protocol.ChallengeResp{
		Signature: protocol.ResolveNonce(i.own.PrivKey, c.Nonce),
	}}
}

// readChallenge reads n handshake frames and returns the CHALLENGE among
// them.
func readChallenge(conn net.Conn, n int) (protocol.Challenge, error) {
	var c protocol.Challenge
	for range n {
		f, err := ReadFrame(conn)
		if err != nil {
			return c, err
		}
		if got, ok := f.Body.(protocol.Challenge); ok {
			c = got
		}
	}
	return c, nil
}

func TestInboundHandshakeRejectsImpostor(t *testing.T) {
	server := newIdentity(t)
	imp := newImpostor(t)

	// Register replaces sessions silently, so a handshake that registered
	// before verifying would let anyone evict a connected peer.
	sm := peer.NewSessionManager()
	legit := peer.NewSession(imp.victim.ID, imp.victim.PubKey, nil)
	sm.Register(legit)

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	go func() {
		_ = UnsafeSendFrame(a, imp.hello())
		_ = UnsafeSendFrame(a, protocol.Frame{Body: protocol.Challenge{}})
		c, err := readChallenge(a, 3)
		if err != nil {
			return
		}
		_ = UnsafeSendFrame(a, imp.answer(c))
	}()

	if _, err := InboundHandshake(b, server, sm); err == nil {
		t.Fatal("accepted a peer that cannot sign for the key it advertised")
	}
	if s, _ := sm.Get(imp.victim.ID); s != legit {
		t.Fatal("failed handshake replaced the impersonated peer's session")
	}
}

func TestOutboundHandshakeRejectsImpostor(t *testing.T) {
	client := newIdentity(t)
	imp := newImpostor(t)
	sm := peer.NewSessionManager()

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	go func() {
		c, err := readChallenge(b, 2)
		if err != nil {
			return
		}
		_ = UnsafeSendFrame(b, imp.hello())
		_ = UnsafeSendFrame(b, protocol.Frame{Body: protocol.Challenge{}})
		_ = UnsafeSendFrame(b, imp.answer(c))
		// A client that wrongly accepted us now sends its own answer;
		// reading it lets the test fail instead of hanging on net.Pipe.
		_, _ = ReadFrame(b)
	}()

	if _, err := OutboundHandshake(a, client, sm); err == nil {
		t.Fatal("accepted a peer that cannot sign for the key it advertised")
	}
	if _, ok := sm.Get(imp.victim.ID); ok {
		t.Fatal("session registered for a peer that failed to authenticate")
	}
}
