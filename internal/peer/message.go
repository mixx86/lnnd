package peer

import "lnn/internal/protocol"

const maxPending = 65536

func (s *SelfNode) RegisterPendingMessage() (<-chan protocol.Payload, uint32, int64) {
	s.PendingMu.Lock()
	defer s.PendingMu.Unlock()
	if len(s.Pending) >= maxPending {
		return nil, 0, protocol.ErrTooManyPending
	}
	var tag uint32
	for {
		tag = s.Tag.Add(1)
		if tag != 0 {
			if _, exists := s.Pending[tag]; !exists {
				break
			}
		}
	}
	ch := make(chan protocol.Payload, 1)
	s.Pending[tag] = ch
	return ch, tag, 0
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
