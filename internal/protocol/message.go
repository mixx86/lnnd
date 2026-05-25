package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

// Message types for the LNN protocol.
const (
	MsgHello           uint8 = 0 // Initial peer identification
	MsgChallenge       uint8 = 1 // Authentication challenge
	MsgChallengeResp   uint8 = 2 // Challenge response with signature
	MsgError           uint8 = 3 // Error notification
	MsgLinkSym         uint8 = 4 // Symbolic link operation
	MsgLinkHard        uint8 = 5 // Hard link operation
	MsgSymLinkRequest  uint8 = 6 // Request to create symbolic link
	MsgHardLinkRequest uint8 = 7 // Request to create hard link
	MsgOk              uint8 = 8 // Success acknowledgment
)

// Message is the interface for all protocol messages.
type Message interface {
	Encode() []byte
}

// InboundRequest marks inbound messages handled as requests.
type InboundRequest interface {
	Message
	RequestType() uint8
}

// OkMessage is a zero-payload acknowledgment sent after a message
// has been successfully handled.
type OkMessage struct{}

func (OkMessage) Encode() []byte {
	packet := make([]byte, HeaderSize)
	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgOk
	return packet
}

// Header represents the fixed-size protocol header. See architecture.md for the full layout.
type Header struct {
	Magic         [4]byte
	Version       uint8
	Type          uint8
	Tag           uint16
	PayloadLength uint32
}

const (
	MagicSize   = 4
	VersionSize = 1
	TypeSize    = 1
	TagSize     = 2
	LenSize     = 4

	OffsetMagic   = 0
	OffsetVersion = 4
	OffsetType    = 5
	OffsetTag     = 6
	OffsetLen     = 8
	OffsetBody    = 12
)
const HeaderSize = MagicSize + VersionSize + TypeSize + LenSize

var protocolMagic = [4]byte{0x4C, 0x4E, 0x4E, 0x00}

const protocolVersion uint8 = 0x01

// ReadHeader reads and validates the protocol header from r.
func ReadHeader(r io.Reader) (Header, error) {
	var header Header
	headerBuf := make([]byte, HeaderSize)
	_, err := io.ReadFull(r, headerBuf)
	if err != nil {
		return header, err
	}

	copy(header.Magic[:], headerBuf[0:4])
	header.Version = headerBuf[4]
	header.Type = headerBuf[5]
	header.PayloadLength = binary.BigEndian.Uint32(headerBuf[6:10])

	var expectedMagic = [4]byte{0x4C, 0x4E, 0x4E, 0x00}
	if header.Magic != expectedMagic {
		return header, errors.New("invalid magic")
	}

	if header.Version != 0x01 {
		return header, errors.New("unsupported version")
	}

	return header, nil
}
