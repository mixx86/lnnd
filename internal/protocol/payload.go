package protocol

import "fmt"

const (
	symlinkRequestMinSize = 3
	symlinkRequestMaxSize = 8192
)

// PayloadLength validates the length announced by the header before any
// payload byte is read, so a hostile peer cannot make us allocate
// arbitrary amounts of memory.
func PayloadLength(h Header) (uint32, error) {
	var want uint32
	switch h.Type {
	case MsgHello:
		want = HelloPayloadSize
	case MsgChallenge:
		want = ChallengePayloadSize
	case MsgChallengeResp:
		want = ChallengeRespPayloadSize
	case MsgError:
		want = ErrorMessagePayloadSize
	case MsgOk, MsgRecoverRequest:
		want = 0
	case MsgSymlinkRequest:
		if h.PayloadLength < symlinkRequestMinSize ||
			h.PayloadLength > symlinkRequestMaxSize {
			return 0, fmt.Errorf(
				"invalid payload size %d for type %d",
				h.PayloadLength,
				h.Type,
			)
		}
		return h.PayloadLength, nil
	default:
		return 0, fmt.Errorf("unknown message type: %d", h.Type)
	}

	if h.PayloadLength != want {
		return 0, fmt.Errorf(
			"invalid payload size %d for type %d: want %d",
			h.PayloadLength,
			h.Type,
			want,
		)
	}
	return want, nil
}

// ParsePayload lives here rather than next to ReadFrame so that anything
// that stores encoded payloads (the outbox) can decode them without
// depending on the network package.
func ParsePayload(t MsgType, payload []byte) (Payload, error) {
	switch t {
	case MsgHello:
		return ParseHello(payload)
	case MsgChallenge:
		return ParseChallenge(payload)
	case MsgChallengeResp:
		return ParseChallengeResp(payload)
	case MsgError:
		return ParseError(payload)
	case MsgOk:
		return OK{}, nil
	case MsgRecoverRequest:
		return RecoverRequest{}, nil
	case MsgSymlinkRequest:
		return ParseSymlinkRequest(payload)
	default:
		return nil, fmt.Errorf("unknown message type: %d", t)
	}
}
