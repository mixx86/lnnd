package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	ErrorMessagePayloadSize = 43
	ErrorMessageSize        = 35 // zero-padded on the wire
	CodeErrorMessageSize    = 8
)

type Error struct {
	Code    int64
	Message string
}

// Payload layout: [code:8][message:35]
func EncodeError(code int64, message string) []byte {
	packet := make([]byte, ErrorMessagePayloadSize)

	binary.BigEndian.PutUint64(packet[:], uint64(code))
	copy(packet[CodeErrorMessageSize:], message)

	return packet
}

func (e Error) Encode() []byte {
	return EncodeError(e.Code, e.Message)
}

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

func (Error) Type() MsgType { return MsgError }
func (Error) Size() uint32  { return ErrorMessagePayloadSize }

const (
	ErrInvalidMessage  int64 = -1
	ErrInvalidState    int64 = -2
	ErrRequestRejected int64 = -3
	ErrOverloaded      int64 = -4
	ErrTooManyPending  int64 = -5
	ErrPendingTimeout  int64 = -6
	ErrInternal        int64 = -7
	ErrUnknownTag      int64 = -8
)

const (
	MsgErrInvalidMessage  = "Invalid message."
	MsgErrInvalidState    = "Invalid state."
	MsgErrRequestRejected = "Request rejected."
	MsgErrOverloaded      = "Daemon overloaded."
	MsgErrPendingTimeout  = "Timed out waiting for a response."
	MsgErrInternal        = "Internal error."
	MsgErrUnknownTag      = "Unknown tag."
)
