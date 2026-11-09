package network

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"lnn/internal/outbox"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/recovery"
)

type pipeTransport struct{ conn net.Conn }

func (p pipeTransport) Listen() (net.Listener, error) {
	return nil, errors.New("not supported")
}

func (p pipeTransport) Dial(context.Context, string) (net.Conn, error) {
	return p.conn, nil
}

// Before Connect, sessions dialed from IPC had no read loop, so the peer's
// OK was never read and the frame stayed in the outbox forever.
func TestConnectReadsAnswersOnOutboundSession(t *testing.T) {
	local := newIdentity(t)
	remote := newIdentity(t)
	dir := t.TempDir()

	ob, err := outbox.Open(filepath.Join(dir, "outbox"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ob.Close() })
	rp, err := recovery.Open(filepath.Join(dir, "requests.recovery"))
	if err != nil {
		t.Fatal(err)
	}

	a, b := net.Pipe()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })

	srv := &Server{
		Transport: pipeTransport{conn: a},
		Sessions:  peer.NewSessionManager(),
		Identity:  local,
		Outbox:    ob,
		Recovery:  rp,
	}

	// The remote peer answers OK to the first frame it gets after the
	// handshake, as it does once a request is stored.
	peerErr := make(chan error, 1)
	go func() {
		_, err := InboundHandshake(b, remote, peer.NewSessionManager())
		if err != nil {
			peerErr <- err
			return
		}
		f, err := ReadFrame(b)
		if err != nil {
			peerErr <- err
			return
		}
		peerErr <- UnsafeSendFrame(b, protocol.Frame{
			Body: protocol.OK{},
		}.WithTag(f.Header.Tag))
	}()

	s, err := srv.Connect(context.Background(), "peer")
	if err != nil {
		t.Fatal(err)
	}
	frame := ob.Put(s.Remote.ID, protocol.SymlinkRequest{Src: "/a"})
	if err := SendFrame(s, frame); err != nil {
		t.Fatal(err)
	}

	// Without a reader the peer's OK blocks on the pipe, so peerErr is only
	// awaited once the outbox shows the OK was read.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := ob.Get(s.Remote.ID, frame.Header.Tag); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("frame still in outbox: the OK was never read")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := <-peerErr; err != nil {
		t.Fatal(err)
	}
}
