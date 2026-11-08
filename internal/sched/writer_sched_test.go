package sched

import (
	"testing"
	"time"
)

func waitingWriters(s *Service) int {
	s.writeTurnMu.Lock()
	defer s.writeTurnMu.Unlock()
	return len(s.waitingWriters)
}

func TestElevatedDiskLoadCapsTurnsAndServesHeaviestFirst(t *testing.T) {
	s := NewService("")
	go s.Run()

	// Inside the 65–90 % band: Elevated, which allows 4 concurrent writers.
	s.UpdateDiskUtilization(80)

	var active []*Writer
	for range 4 {
		active = append(active, s.WaitWriteTurn(RequestsCacheID))
	}

	granted := make(chan SourceID, 3)
	queued := []SourceID{RequestsCacheID, FilesID, RequestsID}
	for i, source := range queued {
		go func() {
			s.WaitWriteTurn(source)
			granted <- source
		}()
		waitUntil(t, "the writer is queued", func() bool {
			return waitingWriters(s) == i+1 || len(granted) > 0
		})
	}

	select {
	case source := <-granted:
		t.Fatalf("source %d got a fifth concurrent turn", source)
	case <-time.After(50 * time.Millisecond):
	}

	for _, want := range []SourceID{FilesID, RequestsID, RequestsCacheID} {
		s.ReleaseWriteTurn(active[0], 0)
		active = active[1:]

		select {
		case got := <-granted:
			if got != want {
				t.Fatalf("freed turn went to source %d, want %d", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("freed turn was never handed to source %d", want)
		}
	}
}
