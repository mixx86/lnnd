package peer

import "lnn/internal/protocol"

const maxPending = 65536

// RegisterPendingMessage takes the tag from the outbox instead of minting
// one: tags must survive reconnects, and a session does not.
func (s *SelfNode) RegisterPendingMessage(
	tag uint32,
) (<-chan protocol.Payload, int64) {
	s.PendingMu.Lock()
	defer s.PendingMu.Unlock()
	if len(s.Pending) >= maxPending {
		return nil, protocol.ErrTooManyPending
	}
	ch := make(chan protocol.Payload, 1)
	s.Pending[tag] = ch
	return ch, 0
}

func (s *SelfNode) ResolveMessage(tag uint32, msg protocol.Payload) {
	s.PendingMu.Lock()
	ch, ok := s.Pending[tag]
	if ok {
		delete(s.Pending, tag)
	}
	s.PendingMu.Unlock()
	if ok {
		ch <- msg
	}
}

func (s *SelfNode) CancelPendingMessage(tag uint32) {
	s.PendingMu.Lock()
	ch, ok := s.Pending[tag]
	if ok {
		delete(s.Pending, tag)
	}
	s.PendingMu.Unlock()
	if ok {
		ch <- protocol.Error{
			Code:    protocol.ErrPendingTimeout,
			Message: protocol.MsgErrPendingTimeout,
		}
	}
}
