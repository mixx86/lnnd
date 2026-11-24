package network

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"lnn/internal/outbox"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/queues"
	"lnn/internal/recovery"
	"lnn/internal/sched"
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
		srv: &Server{
			Outbox:    ob,
			Recovery:  rp,
			Scheduler: sched.NewService(""),
		},
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
		protocol.SymlinkRequest{ReqID: 1, Src: "/a"},
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
	req := protocol.SymlinkRequest{ReqID: 1, Src: "/a"}
	tag := h.srv.Outbox.Put(id, req).Header.Tag

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
			Message: protocol.MsgErrUnknownTag.Error(),
		},
	}.WithTag(5))
	if err != nil {
		t.Fatalf("tagged error closed the connection: %v", err)
	}
	if left := h.srv.Recovery.ForPeer(id); len(left) != 0 {
		t.Fatalf("recovery entries left: %+v", left)
	}
}

// A payload that fails to parse was still read whole, so the stream stays
// in sync: the peer gets an ERROR with the frame's tag and the session
// keeps serving.
func TestInvalidPayloadKeepsSession(t *testing.T) {
	h := newHarness(t)
	go func() {
		serveSession(h.session.Conn, h.session, h.srv)
		_ = h.session.Conn.Close()
	}()

	err := WriteFrame(h.remote, protocol.Frame{
		Body: protocol.SymlinkRequest{ReqID: 1, Src: "relative/path"},
	}.WithTag(7))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(h.remote)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Body.(protocol.Error); !ok || got.Header.Tag != 7 {
		t.Fatalf("got %+v tag %d, want ERROR tag 7",
			got.Body, got.Header.Tag)
	}

	err = WriteFrame(h.remote, protocol.Frame{
		Body: protocol.RecoverRequest{},
	}.WithTag(8))
	if err != nil {
		t.Fatalf("session closed after an invalid payload: %v", err)
	}
	got, err = ReadFrame(h.remote)
	if err != nil {
		t.Fatalf("session closed after an invalid payload: %v", err)
	}
	if got.Header.Tag != 8 {
		t.Fatalf("got reply for tag %d, want 8", got.Header.Tag)
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

// nextQueued plays the requests writer: it takes the request a handler
// submitted to the scheduler.
func nextQueued(t *testing.T, s *sched.Service) queues.PendingRequest {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		pr, ok, err := s.TryNextRequest()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return pr
		}
		if time.Now().After(deadline) {
			t.Fatal("request never reached the scheduler")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSymlinkRequestRepliesOnlyOnceWriteIsSettled(t *testing.T) {
	cases := []struct {
		name     string
		writeErr error
		want     protocol.Payload
		wantOwed int
	}{
		{"durable", nil, protocol.OK{}, 0},
		{
			"write failed",
			errors.New("write failed"),
			protocol.Error{
				Code:    protocol.ErrInternal,
				Message: protocol.MsgErrInternal.Error(),
			},
			1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			id := h.session.Remote.ID
			const tag = 5
			// The tag is owed by a crash recovery: this request is the
			// peer's resend.
			err := h.srv.Recovery.Add(
				recovery.Entry{Peer: id, ReqID: 3, Tag: tag},
			)
			if err != nil {
				t.Fatal(err)
			}

			replies := make(chan protocol.Frame, 1)
			go func() {
				if f, err := ReadFrame(h.remote); err == nil {
					replies <- f
				}
			}()

			req := protocol.Frame{
				Body: protocol.SymlinkRequest{ReqID: 1, Src: "/a"},
			}.WithTag(tag)
			if err := <-h.handle(t, req); err != nil {
				t.Fatal(err)
			}
			queued := nextQueued(t, h.srv.Scheduler)

			select {
			case f := <-replies:
				t.Fatalf("replied %T before the write settled", f.Body)
			case <-time.After(50 * time.Millisecond):
			}
			if owed := h.srv.Recovery.ForPeer(id); len(owed) != 1 {
				t.Fatal("recovery settled before the resend was durable")
			}

			queued.Written <- queues.WriteResult{Err: c.writeErr}

			select {
			case reply := <-replies:
				if reply.Body != c.want || reply.Header.Tag != tag {
					t.Fatalf("got %+v tag %d, want %+v tag %d",
						reply.Body, reply.Header.Tag, c.want, tag)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("no reply once the write settled")
			}
			owed := h.srv.Recovery.ForPeer(id)
			if len(owed) != c.wantOwed {
				t.Fatalf("recovery entries left %+v, want %d",
					owed, c.wantOwed)
			}
		})
	}
}

func TestFramesRoundTrip(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	pubKey := ed25519.PublicKey(bytes.Repeat([]byte{1}, protocol.PublicKeySize))
	bodies := []protocol.Payload{
		protocol.Hello{ID: uuid.New(), PublicKey: pubKey},
		protocol.Challenge{Nonce: [protocol.NonceSize]byte{2}},
		protocol.ChallengeResp{
			Signature: [protocol.ChallengeRespPayloadSize]byte{3},
		},
		protocol.Error{
			Code:    protocol.ErrUnknownTag,
			Message: protocol.MsgErrUnknownTag.Error(),
		},
		protocol.OK{},
		protocol.RecoverRequest{},
		protocol.SymlinkRequest{ReqID: 1, Src: "/a"},
		protocol.SymlinkRequest{ReqID: 2, Src: "/a", Dst: "/b"},
	}
	go func() {
		for i, body := range bodies {
			frame := protocol.Frame{Body: body}.WithTag(uint32(i + 1))
			_ = WriteFrame(local, frame)
		}
	}()

	// Reading them back from one stream also proves no frame carries bytes
	// its header does not announce: they would be read as the next header.
	for i, want := range bodies {
		got, err := ReadFrame(remote)
		if err != nil {
			t.Fatalf("reading %T: %v", want, err)
		}
		if got.Header.Tag != uint32(i+1) ||
			!reflect.DeepEqual(got.Body, want) {
			t.Fatalf("got %+v tag %d, want %+v tag %d",
				got.Body, got.Header.Tag, want, i+1)
		}
	}
}
