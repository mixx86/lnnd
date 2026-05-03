package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	ErrorMessagePayloadSize = 43
	ErrorMessageSize        = 35
	CodeErrorMessageSize    = 8
)

// ErrorMessage signals a protocol error to the remote peer.
type ErrorMessage struct {
	Code    int64
	Message string
}

// EncodeErrorMessage constructs a complete error message packet.
// Payload layout: [code:8][message:35]
func EncodeErrorMessage(code int64, message string) []byte {
	packet := make([]byte, HeaderSize+ErrorMessagePayloadSize)

	// Header
	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgError
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], ErrorMessagePayloadSize)

	// Payload
	binary.BigEndian.PutUint64(packet[OffsetBody:], uint64(code))
	copy(packet[OffsetBody+CodeErrorMessageSize:], message)

	return packet
}

// Encode serializes the error message into wire format.
func (m ErrorMessage) Encode() ([]byte, error) {
	return EncodeErrorMessage(m.Code, m.Message), nil
}

// ParseError deserializes an error message from raw bytes.
func ParseError(payload []byte) (ErrorMessage, error) {
	if len(payload) != ErrorMessagePayloadSize {
		return ErrorMessage{}, errors.New("invalid payload size")
	}
	code := int64(binary.BigEndian.Uint64(payload[:CodeErrorMessageSize]))
	raw := payload[CodeErrorMessageSize:]
	n := 0
	for n < ErrorMessageSize && raw[n] != 0 {
		n++
	}
	return ErrorMessage{Code: code, Message: string(raw[:n])}, nil
}

// Error code constants.
const (
	ErrInvalidMessage int64 = -1
	ErrInvalidState   int64 = -2
)

// Error message string constants.
const (
	MsgErrInvalidMessage = "Invalid message."
	MsgErrInvalidState   = "Invalid state."
)
