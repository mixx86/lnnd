package outbox

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"lnn/internal/protocol"
)

func openTemp(t *testing.T) (*Outbox, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "outbox")
	o, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return o, path
}

func request(src string) protocol.SymlinkRequest {
	return protocol.SymlinkRequest{SenderReqID: 1, Src: src}
}

func TestTagsWrapAroundSkippingZero(t *testing.T) {
	o, _ := openTemp(t)
	defer o.Close()
	peer := uuid.New()

	o.window(peer).lastTag = math.MaxUint32 - 1
	if got := o.Put(peer, request("/a")).Header.Tag; got != math.MaxUint32 {
		t.Fatalf("got tag %d, want %d", got, uint32(math.MaxUint32))
	}
	if got := o.Put(peer, request("/b")).Header.Tag; got != 1 {
		t.Fatalf("got tag %d after wrap, want 1", got)
	}
}

func TestTagsAreIndependentPerPeer(t *testing.T) {
	o, _ := openTemp(t)
	defer o.Close()

	a := o.Put(uuid.New(), request("/a")).Header.Tag
	b := o.Put(uuid.New(), request("/b")).Header.Tag
	if a != 1 || b != 1 {
		t.Fatalf("got tags %d and %d, want 1 and 1", a, b)
	}
}

func TestFullWindowEvictsOldest(t *testing.T) {
	o, _ := openTemp(t)
	defer o.Close()
	peer := uuid.New()

	first := o.Put(peer, request("/first")).Header.Tag
	for i := 0; i < Window; i++ {
		o.Put(peer, request("/x"))
	}

	if _, ok := o.Get(peer, first); ok {
		t.Fatal("oldest frame survived a full window")
	}
	if n := len(o.window(peer).unackedTags); n != Window {
		t.Fatalf("window holds %d frames, want %d", n, Window)
	}
}

func TestAckRemovesFrame(t *testing.T) {
	o, _ := openTemp(t)
	defer o.Close()
	peer := uuid.New()

	tag := o.Put(peer, request("/a")).Header.Tag
	if !o.Ack(peer, tag) {
		t.Fatal("Ack returned false for a stored tag")
	}
	if _, ok := o.Get(peer, tag); ok {
		t.Fatal("frame still present after Ack")
	}
	if o.Ack(peer, tag) {
		t.Fatal("second Ack returned true")
	}
}

func TestReopenRestoresUnconfirmedFrames(t *testing.T) {
	o, path := openTemp(t)
	peer := uuid.New()

	acked := o.Put(peer, request("/acked")).Header.Tag
	kept := o.Put(peer, request("/kept")).Header.Tag
	o.Ack(peer, acked)
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	o, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()

	frame, ok := o.Get(peer, kept)
	if !ok {
		t.Fatal("unconfirmed frame lost on reopen")
	}
	if got := frame.Body.(protocol.SymlinkRequest).Src; got != "/kept" {
		t.Fatalf("restored src %q, want /kept", got)
	}
	if _, ok := o.Get(peer, acked); ok {
		t.Fatal("acked frame came back on reopen")
	}

	next := o.Put(peer, request("/new")).Header.Tag
	if next <= kept+restartTagSkip {
		t.Fatalf("tag %d reused the range before restart (last %d)",
			next, kept)
	}
}

func TestReopenAfterCompactionDoesNotReuseTags(t *testing.T) {
	o, path := openTemp(t)
	peer := uuid.New()

	// A frame the peer never confirms stays in the window while newer
	// ones are confirmed, so the compacted log keeps a frame whose tag
	// lags far behind the last tag handed out.
	o.Put(peer, request("/unconfirmed"))
	var last uint32
	for i := 0; i <= restartTagSkip; i++ {
		last = o.Put(peer, request("/confirmed")).Header.Tag
		o.Ack(peer, last)
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	// Open compacts the log, so the Open after it replays a compacted file.
	o, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	o, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()

	next := o.Put(peer, request("/new")).Header.Tag
	if floor := last + restartTagSkip; next <= floor {
		t.Fatalf("tag %d handed out after a restart, want > %d "+
			"(last tag before the restart %d + %d)",
			next, floor, last, restartTagSkip)
	}
}

func TestReopenIgnoresTornTail(t *testing.T) {
	o, path := openTemp(t)
	peer := uuid.New()
	tag := o.Put(peer, request("/a")).Header.Tag
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0, 0, 0, 90, 1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	o, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if _, ok := o.Get(peer, tag); !ok {
		t.Fatal("valid record lost because of a torn tail")
	}
}
