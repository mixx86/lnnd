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
type ErrorMessage struct {
	Code    int64
	Message string
}

// EncodeErrorMessage constructs a complete error message packet.
// Payload layout: [code:8][message:35]
func EncodeErrorMessage(code int64, message string) []byte {
	packet := make([]byte, HeaderSize+ErrorMessagePayloadSize)

	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgError
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], ErrorMessagePayloadSize)

	binary.BigEndian.PutUint64(packet[OffsetBody:], uint64(code))
	copy(packet[OffsetBody+CodeErrorMessageSize:], message)

	return packet
}

// Encode serializes the error message into wire format.
func (m ErrorMessage) Encode() []byte {
	return EncodeErrorMessage(m.Code, m.Message)
}

// ParseError deserializes an error message from raw bytes.
func ParseError(payload []byte) (ErrorMessage, error) {
	if len(payload) != ErrorMessagePayloadSize {
		return ErrorMessage{}, errors.New("invalid payload size")
	}
	code := int64(binary.BigEndian.Uint64(payload[:CodeErrorMessageSize]))
	raw := payload[CodeErrorMessageSize:]
	n := 0
	// The message field is zero-padded to a fixed size on the wire; scan to the first
	// null byte to recover the original string without trailing garbage.
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
