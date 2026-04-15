// Package peer manages connections to remote peers.
package peer

import (
	"github.com/google/uuid"
)

// NewPeerManager creates a new PeerManager instance.
func NewPeerManager() *PeerManager {
	return &PeerManager{
		peers: make(map[uuid.UUID]*Peer),
	}
}

// Register adds a peer connection to the manager.
func (pm *PeerManager) Register(p *Peer) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.peers[p.ID] = p
}

// Get retrieves a peer connection by ID.
// Returns the peer and true if found, nil and false otherwise.
func (pm *PeerManager) Get(id uuid.UUID) (*Peer, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.peers[id]
	return p, ok
}

// Remove deletes a peer connection from the manager.
func (pm *PeerManager) Remove(id uuid.UUID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, id)
}
