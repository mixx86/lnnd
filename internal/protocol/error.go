package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	ErrorMessagePayloadSize = 43
	ErrorMessageSize        = 35
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
		return Error{}, fmt.Errorf(
			"%w: invalid payload size",
			MsgErrInvalidMessage,
		)
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
	ErrInvalidMessage int64 = -(iota + 1)
	ErrInvalidState
	ErrRequestRejected
	ErrOverloaded
	ErrTooManyPending
	ErrPendingTimeout
	ErrInternal
	ErrUnknownTag
)

var (
	MsgErrInvalidMessage  = errors.New("invalid message")
	MsgErrInvalidState    = errors.New("invalid state")
	MsgErrRequestRejected = errors.New("request rejected")
	MsgErrOverloaded      = errors.New("daemon overloaded")
	MsgErrPendingTimeout  = errors.New("timed out waiting for a response")
	MsgErrInternal        = errors.New("internal error")
	MsgErrUnknownTag      = errors.New("unknown tag")
)
