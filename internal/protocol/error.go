package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	ErrorMessagePayloadSize = 43 // Total payload: 8-byte code + 35-byte message.
	ErrorMessageSize        = 35 // Fixed-size message field; zero-padded on the wire.
	CodeErrorMessageSize    = 8  // int64 error code, big-endian.
)

// ErrorMessage signals a protocol error to the remote peer.
type Error struct {
	Code    int64
	Message string
}

// EncodeErrorMessage constructs a complete error message packet.
// Payload layout: [code:8][message:35]
func EncodeError(code int64, message string) []byte {
	packet := make([]byte, HeaderSize+ErrorMessagePayloadSize)

	binary.BigEndian.PutUint64(packet[:], uint64(code))
	copy(packet[CodeErrorMessageSize:], message)

	return packet
}

// Encode serializes the error message into wire format.
func (e Error) Encode() []byte {
	return EncodeError(e.Code, e.Message)
}

// ParseError deserializes an error message from raw bytes.
func ParseError(payload []byte) (Error, error) {
	if len(payload) != ErrorMessagePayloadSize {
		return Error{}, errors.New("invalid payload size")
	}
	code := int64(binary.BigEndian.Uint64(payload[:CodeErrorMessageSize]))
	raw := payload[CodeErrorMessageSize:]
	n := 0
	// Message field is zero-padded on the wire; stop at the first null byte.
	for n < ErrorMessageSize && raw[n] != 0 {
		n++
	}
	return Error{Code: code, Message: string(raw[:n])}, nil
}

func (Error) Type() MsgKind { return MsgError }
func (Error) Size() uint32  { return ErrorMessageSize }

const (
	ErrInvalidMessage  int64 = -1
	ErrInvalidState    int64 = -2
	ErrRequestRejected int64 = -3
	ErrOverloaded      int64 = -4
	ErrTooManyPending  int64 = -5
	ErrPendingTimeout  int64 = -6
	ErrInternal        int64 = -7
)

const (
	MsgErrInvalidMessage  = "Invalid message."
	MsgErrInvalidState    = "Invalid state."
	MsgErrRequestRejected = "Request rejected."
	MsgErrOverloaded      = "Daemon overloaded."
	MsgErrPendingTimeout  = "Timed out waiting for a response."
	MsgErrInternal        = "Internal error."
)
