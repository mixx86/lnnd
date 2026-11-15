package network

import (
	"context"
	"lnn/internal/peer"
	"lnn/internal/protocol"

	"github.com/google/uuid"
)

func (s *Server) SessionFor(
	ctx context.Context,
	id uuid.UUID,
	addr string,
) (*peer.Session, error) {
	if sess, ok := s.Sessions.Get(id); ok {
		return sess, nil
	}
	return s.Connect(ctx, addr)
}

func (s *Server) RoundTrip(
	ctx context.Context,
	sess *peer.Session,
	p protocol.Payload,
) (protocol.Payload, error) {
	frame := s.Outbox.Put(sess.Remote.ID, p)
	tag := frame.Header.Tag

	ch, err := sess.Self.RegisterPendingMessage(tag)
	if err != nil {
		return nil, err
	}
	if err := SendFrame(sess, frame); err != nil {
		sess.Self.CancelPendingMessage(tag)
		return nil, err
	}

	select {
	case reply := <-ch:
		return reply, nil
	case <-ctx.Done():
		sess.Self.CancelPendingMessage(tag)
		return nil, ctx.Err()
	}
}
