package protocol

import (
	"crypto/ed25519"

	"github.com/google/uuid"
)

const HelloPayloadSize uint32 = 48

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

func (m Hello) Encode() []byte {
	return EncodeHello(m.ID, m.PublicKey)
}

func (Hello) Type() MsgType {
	return MsgHello
}

func (Hello) Size() uint32 {
	return HelloPayloadSize
}

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
