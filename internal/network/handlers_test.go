package network

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"lnn/internal/outbox"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/recovery"
)

type harness struct {
	srv     *Server
	session *peer.Session
	remote  net.Conn
}

func newHarness(t *testing.T) *harness {
	t.Helper()
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

	local, remote := net.Pipe()
	t.Cleanup(func() { _ = local.Close(); _ = remote.Close() })

	return &harness{
		srv:     &Server{Outbox: ob, Recovery: rp},
		session: peer.NewSession(uuid.New(), nil, local),
		remote:  remote,
	}
}

// handle runs the handler concurrently because net.Pipe writes block
// until the other side reads.
func (h *harness) handle(t *testing.T, frame protocol.Frame) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- HandleTCPMessage(h.session.Conn, frame, h.session, h.srv)
	}()
	return done
}

func TestRecoverRequestResendsStoredFrame(t *testing.T) {
	h := newHarness(t)
	sent := h.srv.Outbox.Put(
		h.session.Remote.ID,
		protocol.SymlinkRequest{Src: "/a"},
	)

	done := h.handle(t, protocol.Frame{
		Body: protocol.RecoverRequest{},
	}.WithTag(sent.Header.Tag))

	got, err := ReadFrame(h.remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	req, ok := got.Body.(protocol.SymlinkRequest)
	if !ok || req.Src != "/a" || got.Header.Tag != sent.Header.Tag {
		t.Fatalf("got %T tag %d, want resent request with tag %d",
			got.Body, got.Header.Tag, sent.Header.Tag)
	}
}

func TestRecoverRequestUnknownTag(t *testing.T) {
	h := newHarness(t)

	done := h.handle(t, protocol.Frame{
		Body: protocol.RecoverRequest{},
	}.WithTag(99))

	got, err := ReadFrame(h.remote)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	e, ok := got.Body.(protocol.Error)
	if !ok || e.Code != protocol.ErrUnknownTag || got.Header.Tag != 99 {
		t.Fatalf("got %+v tag %d, want ErrUnknownTag tag 99",
			got.Body, got.Header.Tag)
	}
}

func TestOKAcksOutbox(t *testing.T) {
	h := newHarness(t)
	id := h.session.Remote.ID
	tag := h.srv.Outbox.Put(id, protocol.SymlinkRequest{Src: "/a"}).Header.Tag

	if err := <-h.handle(t, protocol.Frame{
		Body: protocol.OK{},
	}.WithTag(tag)); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.srv.Outbox.Get(id, tag); ok {
		t.Fatal("frame still in outbox after OK")
	}
}

func TestUnknownTagErrorDropsRecoveryAndKeepsConnection(t *testing.T) {
	h := newHarness(t)
	id := h.session.Remote.ID
	if err := h.srv.Recovery.Add(
		recovery.Entry{Peer: id, ReqID: 3, Tag: 5},
	); err != nil {
		t.Fatal(err)
	}

	err := <-h.handle(t, protocol.Frame{
		Body: protocol.Error{
			Code:    protocol.ErrUnknownTag,
			Message: protocol.MsgErrUnknownTag,
		},
	}.WithTag(5))
	if err != nil {
		t.Fatalf("tagged error closed the connection: %v", err)
	}
	if left := h.srv.Recovery.ForPeer(id); len(left) != 0 {
		t.Fatalf("recovery entries left: %+v", left)
	}
}

func TestRequestRecoveriesSendsOwedTags(t *testing.T) {
	h := newHarness(t)
	id := h.session.Remote.ID
	if err := h.srv.Recovery.Add(
		recovery.Entry{Peer: id, ReqID: 1, Tag: 10},
		recovery.Entry{Peer: id, ReqID: 2, Tag: 20},
		recovery.Entry{Peer: uuid.New(), ReqID: 3, Tag: 30},
	); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		requestRecoveries(h.srv, h.session)
		close(done)
	}()

	got := map[uint32]bool{}
	for range 2 {
		f, err := ReadFrame(h.remote)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := f.Body.(protocol.RecoverRequest); !ok {
			t.Fatalf("got %T, want RecoverRequest", f.Body)
		}
		got[f.Header.Tag] = true
	}
	<-done

	if !got[10] || !got[20] || len(got) != 2 {
		t.Fatalf("recover requests for tags %v, want 10 and 20", got)
	}
	// Entries are only dropped once the peer answers.
	if left := h.srv.Recovery.ForPeer(id); len(left) != 2 {
		t.Fatalf("entries left: %+v", left)
	}
}

func TestErrorFrameRoundTrip(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	frames := []protocol.Frame{
		protocol.Frame{Body: protocol.Error{
			Code:    protocol.ErrUnknownTag,
			Message: protocol.MsgErrUnknownTag,
		}}.WithTag(7),
		protocol.Frame{Body: protocol.OK{}}.WithTag(8),
	}
	go func() {
		for _, f := range frames {
			_ = WriteFrame(local, f)
		}
	}()

	// Reading both back proves the error frame carries no trailing bytes
	// that would be mistaken for the next header.
	for _, want := range frames {
		got, err := ReadFrame(remote)
		if err != nil {
			t.Fatal(err)
		}
		if got.Header.Tag != want.Header.Tag ||
			got.Body.Type() != want.Body.Type() {
			t.Fatalf("got type %d tag %d, want type %d tag %d",
				got.Body.Type(), got.Header.Tag,
				want.Body.Type(), want.Header.Tag)
		}
	}
}
