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
	MsgLinkSymRequest  uint8 = 6 // Request to create symbolic link
	MsgLinkHardRequest uint8 = 7 // Request to create hard link
	MsgOk              uint8 = 8 // Success acknowledgment
)

// Message is the interface for all protocol messages.
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

// Header field sizes and offsets.
const (
	MagicSize   = 4
	VersionSize = 1
	TypeSize    = 1
	LenSize     = 4

	OffsetMagic   = 0
	OffsetVersion = 4
	OffsetType    = 5
	OffsetLen     = 6
	OffsetBody    = 10
)
const HeaderSize = MagicSize + VersionSize + TypeSize + LenSize

var protocolMagic = [4]byte{0x4C, 0x4E, 0x4E, 0x00}

const protocolVersion uint8 = 0x01

// ReadHeader reads and validates a fixed-size protocol header from r.
// Returns an error if the magic bytes do not match (non-LNN traffic) or
// if the version is unsupported, so callers can reject bad connections early.
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

	// Reject non-LNN traffic before consuming any more of the stream.
	var expectedMagic = [4]byte{0x4C, 0x4E, 0x4E, 0x00}
	if header.Magic != expectedMagic {
		return header, errors.New("invalid magic")
	}

	// Reject connections from incompatible protocol versions before parsing the body.
	if header.Version != 0x01 {
		return header, errors.New("unsupported version")
	}

	return header, nil
}
