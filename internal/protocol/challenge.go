// Package protocol implements the LNN wire protocol messages and encoding.
package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	UUIDSize      = 16
	PublicKeySize = 32
	NonceSize     = 32
)

// ChallengePayloadSize is the size in bytes of a challenge message payload.
const ChallengePayloadSize uint32 = 32

// ChallengeMessage contains a cryptographic nonce for peer authentication.
type ChallengeMessage struct {
	Nonce [32]byte
}

// Encode serializes the challenge message into wire format.
func (m ChallengeMessage) Encode() []byte {
	return EncodeChallenge(m.Nonce)
}

// ParseChallenge deserializes a challenge message from raw bytes.
func ParseChallenge(payload []byte) (ChallengeMessage, error) {
	if len(payload) != int(ChallengePayloadSize) {
		return ChallengeMessage{}, errors.New("invalid payload size")
	}
	var parsedPayload ChallengeMessage
	copy(parsedPayload.Nonce[:], payload)

	return parsedPayload, nil
}

// EncodeChallenge constructs a complete challenge message packet.
func EncodeChallenge(nonce [NonceSize]byte) []byte {
	packet := make([]byte, HeaderSize+ChallengePayloadSize)

	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgChallenge
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], ChallengePayloadSize)

	copy(packet[OffsetBody:], nonce[:])

	return packet
}

// ChallengeRespPayloadSize is the size in bytes of a challenge response (Ed25519 signature).
const ChallengeRespPayloadSize uint32 = 64

// ChallengeRespMessage contains a signature over the received nonce.
type ChallengeRespMessage struct {
	Signature [ChallengeRespPayloadSize]byte
}

// Encode serializes the challenge response message into wire format.
func (m ChallengeRespMessage) Encode() []byte {
	return EncodeChallengeResp(m.Signature)
}

// EncodeChallengeResp constructs a complete challenge response message packet.
func EncodeChallengeResp(sig [ChallengeRespPayloadSize]byte) []byte {
	packet := make([]byte, HeaderSize+ChallengeRespPayloadSize)

	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgChallengeResp
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], ChallengeRespPayloadSize)

	copy(packet[OffsetBody:], sig[:])

	return packet
}

// ParseChallengeResp deserializes a challenge response message from raw bytes.
func ParseChallengeResp(payload []byte) (ChallengeRespMessage, error) {
	if len(payload) != int(ChallengeRespPayloadSize) {
		return ChallengeRespMessage{}, errors.New("invalid payload size")
	}
	var parsedPayload ChallengeRespMessage
	copy(parsedPayload.Signature[:], payload)

	return parsedPayload, nil
}
