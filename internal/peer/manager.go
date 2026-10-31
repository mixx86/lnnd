// Package peer manages connections to remote peers.
package peer

import (
	"github.com/google/uuid"
)

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[uuid.UUID]*Session),
	}
}

// Register adds or replaces a session. Silent overwrite is correct on reconnect.
func (sm *SessionManager) Register(s *Session) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[s.Remote.ID] = s
}

// Get retrieves a session connection by ID. Returns nil, false if not found.
func (sm *SessionManager) Get(id uuid.UUID) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[id]
	return s, ok
}

// Remove deletes a session connection from the manager.
func (sm *SessionManager) Remove(id uuid.UUID) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}
