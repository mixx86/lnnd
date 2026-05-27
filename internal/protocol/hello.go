package protocol

import (
	"crypto/ed25519"

	"github.com/google/uuid"
)

// HelloPayloadSize is the size in bytes of a hello message payload (UUID + public key).
const HelloPayloadSize uint32 = 48

// HelloMessage contains peer identification information.
type HelloMessage struct {
	ID        uuid.UUID
	PublicKey ed25519.PublicKey
}

// EncodeHello constructs a complete hello message packet.
func EncodeHello(id uuid.UUID, pubKey ed25519.PublicKey) []byte {
	packet := make([]byte, HeaderSize+HelloPayloadSize)

	EncodeHeader(packet, MsgHello, 0, HelloPayloadSize)

	cursor := OffsetBody
	copy(packet[cursor:cursor+UUIDSize], id[:])
	cursor += UUIDSize
	copy(packet[cursor:cursor+PublicKeySize], pubKey)

	return packet
}

// Encode serializes the hello message into wire format.
func (m HelloMessage) Encode() []byte {
	return EncodeHello(m.ID, m.PublicKey)
}

// ParseHello deserializes a hello message from raw bytes.
func ParseHello(payload []byte) (HelloMessage, error) {
	var parsedPayload HelloMessage
	cursor := 0

	parsedID, err := uuid.FromBytes(payload[cursor : cursor+UUIDSize])
	if err != nil {
		return HelloMessage{}, err
	}
	cursor += UUIDSize

	pubKey := make(ed25519.PublicKey, PublicKeySize)
	copy(pubKey, payload[cursor:cursor+PublicKeySize])

	parsedPayload.ID = parsedID
	parsedPayload.PublicKey = pubKey
	return parsedPayload, nil
}
