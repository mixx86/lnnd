package network

import (
	"bytes"
	"crypto/ed25519"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"lnn/internal/protocol"
)

func symlinkRequestFrame(payload []byte) []byte {
	frame := protocol.Header{}.Encode(
		protocol.MsgSymlinkRequest,
		uint32(len(payload)),
	)
	return append(frame, payload...)
}

// ReadFrame parses the payload of every frame before the handshake checks
// its type, so whatever it fails to reject is reachable by any host that
// can open a TCP connection.
func FuzzReadFrame(f *testing.F) {
	pubKey := make(ed25519.PublicKey, protocol.PublicKeySize)
	seeds := []protocol.Payload{
		protocol.Hello{ID: uuid.New(), PublicKey: pubKey},
		protocol.Challenge{},
		protocol.ChallengeResp{},
		protocol.Error{
			Code:    protocol.ErrOverloaded,
			Message: protocol.MsgErrOverloaded.Error(),
		},
		protocol.OK{},
		protocol.RecoverRequest{},
		protocol.SymlinkRequest{Src: "/a"},
		protocol.SymlinkRequest{Src: "/a", Dst: "/b"},
	}
	for i, body := range seeds {
		f.Add(protocol.Frame{Body: body}.WithTag(uint32(i)).Encode())
	}
	// src_len 0xffff wraps 2+src_len around in uint16 arithmetic.
	f.Add(symlinkRequestFrame([]byte{0xff, 0xff, '/'}))
	// dst_len points past the end of the payload.
	f.Add(symlinkRequestFrame([]byte{0, 1, '/', 0xff, 0xff, '/'}))

	f.Fuzz(func(t *testing.T, data []byte) {
		frame, err := ReadFrame(bytes.NewReader(data))
		if err != nil {
			return
		}
		// Accepted payloads are kept and sent again re-encoded (requests
		// WAL, outbox), so the encoding must read back as the same message.
		again, err := ReadFrame(bytes.NewReader(frame.Encode()))
		if err != nil {
			t.Fatalf("re-encoded %T does not parse: %v", frame.Body, err)
		}
		if again.Header.Tag != frame.Header.Tag ||
			!reflect.DeepEqual(again.Body, frame.Body) {
			t.Fatalf("%+v read back as %+v", frame, again)
		}
	})
}
