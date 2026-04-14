package peer

import (
	"sync"

	"github.com/google/uuid"
)

type PeerManager struct {
	mu    sync.RWMutex
	peers map[uuid.UUID]*PeerConn
}

func NewPeerManager() *PeerManager {
	return &PeerManager{
		peers: make(map[uuid.UUID]*PeerConn),
	}
}

func (pm *PeerManager) Register(p *PeerConn) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.peers[p.ID] = p
}

func (pm *PeerManager) Get(id uuid.UUID) (*PeerConn, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.peers[id]
	return p, ok
}

func (pm *PeerManager) Remove(id uuid.UUID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, id)
}
