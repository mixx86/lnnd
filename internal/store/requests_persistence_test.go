package store

import (
	"bytes"
	"os"
	"path/filepath"
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

// A request of ours shares the log with the ones peers send: it must get
// its own req_id, and the startup repair must not take its entry for a
// corrupt one and cut it off.
func TestSentRequestSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests")

	s := runningScheduler()
	w := &Writers{RequestsWriter: *NewRequestsWriter(dir, openPending(t, dir))}
	go w.Run(s)

	remote := peer.NewRemoteNode(uuid.New(), nil)
	if err := s.SubmitRequest(
		remote,
		protocol.SymlinkRequest{ReqID: 5, Src: "/srv/received"},
		1,
	); err != nil {
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
	if !bytes.Contains(before, sent.Encode()[protocol.RequestIDLen:]) {
		t.Fatal("sent request is not in the WAL")
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
