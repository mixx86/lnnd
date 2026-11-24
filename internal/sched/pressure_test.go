package sched

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/queues"
)

// testRequest is sized so that a single queued request reaches the byte
// limit set by newPressureService.
var testRequest = protocol.SymlinkRequest{Src: "/" + strings.Repeat("a", 63)}

func newPressureService(t *testing.T) *Service {
	t.Helper()
	t.Setenv("LNND_REQ_QUEUE_MAX_BYTES", strconv.Itoa(int(testRequest.Size())))
	return NewService("")
}

func submitAsync(s *Service, tag uint32) <-chan error {
	done := make(chan error, 1)
	go func() {
		from := peer.NewRemoteNode(uuid.New(), nil)
		done <- s.SubmitRequest(from, testRequest, tag)
	}()
	return done
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func pressured(s *Service) func() bool {
	return func() bool { return s.IsBackPressured(RequestsID) }
}

func receive(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("SubmitRequest did not return")
		return nil
	}
}

// writeQueued plays the requests writer: it takes the next queued request
// and reports it as durable.
func writeQueued(t *testing.T, s *Service) uint32 {
	t.Helper()
	pr, err := s.NextRequest()
	if err != nil {
		t.Fatal(err)
	}
	pr.Written <- queues.WriteResult{}
	return pr.Tag
}

func TestRequestsQueuePressureFollowsQueuedBytes(t *testing.T) {
	s := newPressureService(t)

	first := submitAsync(s, 1)
	waitUntil(t, "a full queue raises pressure", pressured(s))

	writeQueued(t, s)
	if s.IsBackPressured(RequestsID) {
		t.Fatal("pressure still raised after the queue was drained")
	}
	if err := receive(t, first); err != nil {
		t.Fatal(err)
	}

	second := submitAsync(s, 2)
	waitUntil(t, "pressure is raised again", pressured(s))
	writeQueued(t, s)
	if err := receive(t, second); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitRequestWaitsForPressureToClear(t *testing.T) {
	s := newPressureService(t)

	first := submitAsync(s, 1)
	waitUntil(t, "a full queue raises pressure", pressured(s))
	held := submitAsync(s, 2)
	select {
	case err := <-held:
		t.Fatalf("pressured submitter returned (%v) instead of waiting", err)
	case <-time.After(50 * time.Millisecond):
	}

	if tag := writeQueued(t, s); tag != 1 {
		t.Fatalf("writer got tag %d first, want 1", tag)
	}
	if err := receive(t, first); err != nil {
		t.Fatal(err)
	}

	// NextRequest blocks until the held submitter gets its request in.
	if tag := writeQueued(t, s); tag != 2 {
		t.Fatalf("writer got tag %d, want the held request (2)", tag)
	}
	if err := receive(t, held); err != nil {
		t.Fatalf("held request failed once pressure cleared: %v", err)
	}
}

func TestSubmitRequestFailsWithErrOverloadedUnderSustainedPressure(
	t *testing.T,
) {
	s := newPressureService(t)

	first := submitAsync(s, 1)
	waitUntil(t, "a full queue raises pressure", pressured(s))

	from := peer.NewRemoteNode(uuid.New(), nil)
	err := s.SubmitRequest(from, testRequest, 2)
	if !errors.Is(err, ErrOverloaded) {
		t.Fatalf("got %v, want ErrOverloaded", err)
	}

	// The peer is told the refused request was not stored, so it must
	// not reach the writer either.
	if tag := writeQueued(t, s); tag != 1 {
		t.Fatalf("writer got tag %d, want 1", tag)
	}
	if pr, ok, _ := s.TryNextRequest(); ok {
		t.Fatalf("refused request (tag %d) was queued anyway", pr.Tag)
	}
	if err := receive(t, first); err != nil {
		t.Fatal(err)
	}
}
