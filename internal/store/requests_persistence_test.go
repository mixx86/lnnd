package store

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/queues"
	"lnn/internal/recovery"
	"lnn/internal/sched"
)

func openPending(t *testing.T, dir string) *recovery.Pending {
	t.Helper()
	p, err := recovery.Open(filepath.Join(dir, "requests.recovery"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func runningScheduler() *sched.Service {
	s := sched.NewService("")
	go s.Run()
	return s
}

func TestAcknowledgedRequestsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests")

	s := runningScheduler()
	w := &Writers{RequestsWriter: *NewRequestsWriter(dir, openPending(t, dir))}
	go w.Run(s)

	from := peer.NewRemoteNode(uuid.New(), nil)
	reqs := []protocol.SymlinkRequest{
		{ReqID: 7, Src: "/srv/a"},
		{ReqID: 8, Src: "/srv/b", Dst: "/home/user/b"},
	}
	// One at a time, so each request is committed under its own
	// checkpoint and the second checkpoint has to chain onto the first.
	for i, req := range reqs {
		if err := s.SubmitRequest(from, req, uint32(i+1)); err != nil {
			t.Fatalf("submit %+v: %v", req, err)
		}
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range reqs {
		if !bytes.Contains(before, req.Encode()[protocol.RequestIDLen:]) {
			t.Fatalf("acknowledged request %+v is not in the WAL", req)
		}
	}

	pending := openPending(t, dir)
	restarted := NewRequestsWriter(dir, pending)
	if err := restarted.repairOnStartup(); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("startup repair rewrote a WAL holding only acknowledged "+
			"requests: %d bytes before, %d after", len(before), len(after))
	}
	if owed := pending.ForPeer(from.ID); len(owed) != 0 {
		t.Fatalf("acknowledged requests treated as lost: %+v", owed)
	}
	id, err := restarted.reqFile.getReqID()
	if err != nil {
		t.Fatal(err)
	}
	if id != uint32(len(reqs)) {
		t.Fatalf("persisted request id counter %d, want %d: ids would be "+
			"handed out again after the restart", id, len(reqs))
	}
}

type walRequest struct {
	reqID    uint32
	rmtReqID uint32
	payload  []byte
}

// requestEntries decodes the request entries of a WAL by the layout in
// docs/architecture.md, not by the writer's code: header (16 B), then
// [entry_length:4][op:1] and, for requests,
// [req_type:2][peer_uuid:16][req_id:4][rmt_req_id:4][unix_ts:8][payload].
func requestEntries(t *testing.T, wal []byte) []walRequest {
	t.Helper()
	var entries []walRequest
	for off := 16; off < len(wal); {
		n := int(binary.BigEndian.Uint32(wal[off:]))
		if n < 5 || off+n > len(wal) {
			t.Fatalf("entry at offset %d has length %d", off, n)
		}
		if opKind(wal[off+4]) == opRequest {
			fields := wal[off+5 : off+n]
			entries = append(entries, walRequest{
				reqID:    binary.BigEndian.Uint32(fields[18:]),
				rmtReqID: binary.BigEndian.Uint32(fields[22:]),
				payload:  fields[34:],
			})
		}
		off += n
	}
	return entries
}

// A request of ours shares the log with the ones peers send, and only
// rmt_req_id tells them apart. Each entry must keep the sender's id where a
// RESOLUTION will look for it: the peer's id in rmt_req_id for a request it
// sent, the entry's own req_id for ours, whose rmt_req_id stays 0.
func TestSentRequestSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests")

	s := runningScheduler()
	w := &Writers{RequestsWriter: *NewRequestsWriter(dir, openPending(t, dir))}
	go w.Run(s)

	remote := peer.NewRemoteNode(uuid.New(), nil)
	received := protocol.SymlinkRequest{ReqID: 5, Src: "/srv/received"}
	if err := s.SubmitRequest(remote, received, 1); err != nil {
		t.Fatal(err)
	}
	sent := protocol.SymlinkRequest{Src: "/srv/sent", Dst: "/home/peer/l"}
	reqID, err := s.SubmitSentRequest(remote, sent)
	if err != nil {
		t.Fatal(err)
	}
	if reqID != 2 {
		t.Fatalf("sent request got req_id %d, want 2", reqID)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := requestEntries(t, before)
	want := []walRequest{
		{
			reqID:    1,
			rmtReqID: 5,
			payload:  received.Encode()[protocol.RequestIDLen:],
		},
		{
			reqID:    2,
			rmtReqID: 0,
			payload:  sent.Encode()[protocol.RequestIDLen:],
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request entries:\ngot  %+v\nwant %+v", got, want)
	}

	pending := openPending(t, dir)
	if err := NewRequestsWriter(dir, pending).repairOnStartup(); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("startup repair rewrote a WAL holding a sent request: "+
			"%d bytes before, %d after", len(before), len(after))
	}
}

func TestWriterNotBlockedByAbandonedSubmitter(t *testing.T) {
	dir := t.TempDir()
	w := NewRequestsWriter(dir, openPending(t, dir))
	s := runningScheduler()

	// Built by the queue exactly as for SubmitRequest, which stops
	// listening on Written after its confirmation timeout: from then on
	// nobody will ever receive the writer's result.
	abandoned, ok := queues.NewRequestsQueue().TryEnqueueWithTimeout(
		peer.NewRemoteNode(uuid.New(), nil),
		protocol.SymlinkRequest{Src: "/a"},
		1,
		false,
		time.Second,
	)
	if !ok {
		t.Fatal("enqueue failed")
	}

	done := make(chan error, 1)
	go func() {
		done <- w.persistRequests([]queues.PendingRequest{abandoned}, s)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer blocked reporting to a submitter that gave up; " +
			"no later request could ever be persisted")
	}
}
