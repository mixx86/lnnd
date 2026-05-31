package protocol

import (
	"crypto/ed25519"

	"github.com/google/uuid"
)

// HelloPayloadSize is the size in bytes of a hello message payload (UUID + public key).
const HelloPayloadSize uint32 = 48

// Hello contains peer identification information.
type Hello struct {
	ID        uuid.UUID
	PublicKey ed25519.PublicKey
}

func EncodeHello(id uuid.UUID, pubKey ed25519.PublicKey) []byte {
	packet := make([]byte, HelloPayloadSize)
	copy(packet[:UUIDSize], id[:])
	copy(packet[UUIDSize:UUIDSize+PublicKeySize], pubKey)
	return packet
}

// Encode serializes the hello message into wire format.
func (m Hello) Encode() []byte {
	return EncodeHello(m.ID, m.PublicKey)
}

func (Hello) Type() uint8 {
	return MsgHello
}

func (Hello) Size() uint32 {
	return HelloPayloadSize
}

// ParseHello deserializes a hello message from raw bytes.
func ParseHello(payload []byte) (Hello, error) {
	var parsedPayload Hello
	cursor := 0

	parsedID, err := uuid.FromBytes(payload[cursor : cursor+UUIDSize])
	if err != nil {
		return Hello{}, err
	}
	cursor += UUIDSize

	pubKey := make(ed25519.PublicKey, PublicKeySize)
	copy(pubKey, payload[cursor:cursor+PublicKeySize])

	parsedPayload.ID = parsedID
	parsedPayload.PublicKey = pubKey
	return parsedPayload, nil
}
