package protocol

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"

	"github.com/google/uuid"
)

// HelloPayloadSize is the size in bytes of a hello message payload (UUID + public key).
const HelloPayloadSize uint32 = 48

// HelloMessage contains peer identification information.
type HelloMessage struct {
	ID        uuid.UUID         // Peer's unique identifier
	PublicKey ed25519.PublicKey // Peer's public key
}

// EncodeHello constructs a complete hello message packet.
func EncodeHello(id uuid.UUID, pubKey ed25519.PublicKey) []byte {
	packet := make([]byte, HeaderSize+HelloPayloadSize)

	// Header
	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgHello
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], HelloPayloadSize)

	// Payload
	cursor := OffsetBody
	copy(packet[cursor:cursor+UUIDSize], id[:])
	cursor += UUIDSize
	copy(packet[cursor:cursor+PublicKeySize], pubKey)
	// cursor += PublicKeySize <- can be ommited (last field)

	return packet
}

// Encode serializes the hello message into wire format.
func (m HelloMessage) Encode() ([]byte, error) {
	return EncodeHello(m.ID, m.PublicKey), nil
}

// ParseHello deserializes a hello message from raw bytes.
func ParseHello(payload []byte) (HelloMessage, error) {
	if len(payload) != int(HelloPayloadSize) {
		return HelloMessage{}, errors.New("invalid payload size")
	}
	var parsedPayload HelloMessage
	cursor := 0

	parsedID, err := uuid.FromBytes(payload[cursor : cursor+UUIDSize])
	if err != nil {
		return HelloMessage{}, err
	}
	cursor += UUIDSize

	pubKey := make(ed25519.PublicKey, PublicKeySize)
	copy(pubKey, payload[cursor:cursor+PublicKeySize])
	// cursor += PublicKeySize <- can be ommited (last field)

	parsedPayload.ID = parsedID
	parsedPayload.PublicKey = pubKey
	return parsedPayload, nil
}
