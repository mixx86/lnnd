package peer

import (
	"github.com/google/uuid"
)

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[uuid.UUID]*Session),
	}
}

// Register overwrites silently on purpose: a reconnecting peer must replace
// its stale session.
func (sm *SessionManager) Register(s *Session) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[s.Remote.ID] = s
}

func (sm *SessionManager) Get(id uuid.UUID) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[id]
	return s, ok
}

func (sm *SessionManager) Remove(id uuid.UUID) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}
