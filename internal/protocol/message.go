package protocol

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/google/uuid"
)

// Messages types
const (
	MsgHello           uint8 = 0
	MsgChallenge       uint8 = 1
	MsgChallengeResp   uint8 = 2
	MsgError           uint8 = 3
	MsgLinkSym         uint8 = 4
	MsgLinkHard        uint8 = 5
	MsgLinkSymRequest  uint8 = 6
	MsgLinkHardRequest uint8 = 7
	MsgOk              uint8 = 8
)

type Message interface {
	Encode() ([]byte, error)
}

// Header represents the fixed-size protocol header.
// Layout:
// [0-3] Magic
// [4]   Version
// [5]   Type
// [6-9] PayloadLength
type Header struct {
	Magic         [4]byte
	Version       uint8
	Type          uint8
	PayloadLength uint32
}

const (
	// Header sizes within a frame
	MagicSize   = 4
	VersionSize = 1
	TypeSize    = 1
	LenSize     = 4

	// Byte offsets within a frame
	OffsetMagic   = 0
	OffsetVersion = 4
	OffsetType    = 5
	OffsetLen     = 6
	OffsetBody    = 10
)
const HeaderSize = MagicSize + VersionSize + TypeSize + LenSize

// HelloMessage
const HelloPayloadSize uint32 = 48
const (
	UUIDSize      = 16
	PublicKeySize = 32
	NonceSize     = 32
)

type HelloMessage struct {
	ID        uuid.UUID
	PublicKey ed25519.PublicKey
}

func (m HelloMessage) Encode() ([]byte, error) {
	return EncodeHello(m.ID, m.PublicKey), nil
}

var protocolMagic = [4]byte{0x4C, 0x4E, 0x4E, 0x00}

const protocolVersion uint8 = 0x01

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

func ReadHeader(conn net.Conn) (Header, error) {
	var header Header
	headerBuf := make([]byte, HeaderSize)
	_, err := io.ReadFull(conn, headerBuf)
	if err != nil {
		return header, err
	}

	// Magic
	copy(header.Magic[:], headerBuf[0:4])

	// Version
	header.Version = headerBuf[4]

	// Type
	header.Type = headerBuf[5]

	// Length
	header.PayloadLength = binary.BigEndian.Uint32(headerBuf[6:10])

	// Validate magic
	var expectedMagic = [4]byte{0x4C, 0x4E, 0x4E, 0x00}
	if header.Magic != expectedMagic {
		return header, errors.New("invalid magic")
	}

	// Validate version
	if header.Version != 0x01 {
		return header, errors.New("unsupported version")
	}

	return header, nil
}

const ChallengePayloadSize uint32 = 32

type ChallengeMessage struct {
	Nonce [32]byte
}

func (m ChallengeMessage) Encode() ([]byte, error) {
	return EncodeChallenge(m.Nonce), nil
}

func ParseChallenge(payload []byte) (ChallengeMessage, error) {
	if len(payload) != int(ChallengePayloadSize) {
		return ChallengeMessage{}, errors.New("invalid payload size")
	}
	var parsedPayload ChallengeMessage
	copy(parsedPayload.Nonce[:], payload)

	return parsedPayload, nil
}

func EncodeChallenge(nonce [NonceSize]byte) []byte {
	packet := make([]byte, HeaderSize+ChallengePayloadSize)

	// Header
	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgChallenge
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], ChallengePayloadSize)

	// Payload
	copy(packet[OffsetBody:], nonce[:])

	return packet
}

const ChallengeRespPayloadSize uint32 = 64

type ChallengeRespMessage struct {
	Signature [ChallengeRespPayloadSize]byte
}

func (m ChallengeRespMessage) Encode() ([]byte, error) {
	return EncodeChallengeResp(m.Signature), nil
}

func EncodeChallengeResp(sig [ChallengeRespPayloadSize]byte) []byte {
	packet := make([]byte, HeaderSize+ChallengeRespPayloadSize)

	// Header
	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgChallengeResp
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], ChallengeRespPayloadSize)

	// Payload
	copy(packet[OffsetBody:], sig[:])

	return packet
}

func ParseChallengeResp(payload []byte) (ChallengeRespMessage, error) {
	if len(payload) != int(ChallengeRespPayloadSize) {
		return ChallengeRespMessage{}, errors.New("invalid payload size")
	}
	var parsedPayload ChallengeRespMessage
	copy(parsedPayload.Signature[:], payload)

	return parsedPayload, nil
}
