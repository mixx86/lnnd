package recovery

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestPendingSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.recovery")
	peer := uuid.New()

	p, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Add(
		Entry{Peer: peer, LCLReqID: 1, Tag: 10},
		Entry{Peer: peer, LCLReqID: 2, Tag: 20},
	); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := p.Resolve(peer, 10); err != nil || !ok {
		t.Fatalf("Resolve: ok=%v err=%v", ok, err)
	}

	p, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := p.ForPeer(peer)
	if len(got) != 1 || got[0] != (Entry{Peer: peer, LCLReqID: 2, Tag: 20}) {
		t.Fatalf("after reopen got %+v", got)
	}
}

func TestResolveUnknownTag(t *testing.T) {
	p, err := Open(filepath.Join(t.TempDir(), "requests.recovery"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := p.Resolve(uuid.New(), 1); ok || err != nil {
		t.Fatalf("Resolve on empty: ok=%v err=%v", ok, err)
	}
}
