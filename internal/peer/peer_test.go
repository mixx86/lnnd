package peer

import (
	"testing"

	"github.com/google/uuid"
)

func newPeer() *Peer {
	return &Peer{
		ID:    uuid.New(),
		State: StateVerified,
	}
}

func TestRegisterAndGet(t *testing.T) {
	pm := NewPeerManager()
	p := newPeer()
	pm.Register(p)

	got, ok := pm.Get(p.ID)
	if !ok {
		t.Fatal("expected peer to be found after Register")
	}
	if got.ID != p.ID {
		t.Errorf("ID mismatch: got %v, want %v", got.ID, p.ID)
	}
}

func TestGet_notFound(t *testing.T) {
	pm := NewPeerManager()
	_, ok := pm.Get(uuid.New())
	if ok {
		t.Fatal("expected not found for unknown ID")
	}
}

func TestRemove(t *testing.T) {
	pm := NewPeerManager()
	p := newPeer()
	pm.Register(p)
	pm.Remove(p.ID)

	_, ok := pm.Get(p.ID)
	if ok {
		t.Fatal("expected peer to be gone after Remove")
	}
}

func TestRemove_nonExistent(t *testing.T) {
	pm := NewPeerManager()
	pm.Remove(uuid.New()) // should not panic
}

func TestRegister_overwrite(t *testing.T) {
	pm := NewPeerManager()
	p := newPeer()
	pm.Register(p)

	p2 := &Peer{ID: p.ID, State: StateClosing}
	pm.Register(p2)

	got, _ := pm.Get(p.ID)
	if got.State != StateClosing {
		t.Errorf("expected overwritten state %v, got %v", StateClosing, got.State)
	}
}

func TestRegister_multiplePeers(t *testing.T) {
	pm := NewPeerManager()
	peers := make([]*Peer, 5)
	for i := range peers {
		peers[i] = newPeer()
		pm.Register(peers[i])
	}
	for _, p := range peers {
		if _, ok := pm.Get(p.ID); !ok {
			t.Errorf("peer %v not found", p.ID)
		}
	}
}
