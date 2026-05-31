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

type Frame struct {
	Header Header
	Body   Payload
}

func (m Frame) Encode() []byte {
	packet := m.Header.Encode(m.Body.Type(), m.Body.Size())
	packet = append(packet, m.Body.Encode()...)
	return packet
}

func (m Frame) WithTag(tag uint16) Frame {
	m.Header.Tag = tag
	return m
}

func (h Header) Encode(msgType uint8, payloadLen uint32) []byte {
	buf := make([]byte, HeaderSize)
	copy(buf[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	buf[OffsetVersion] = protocolVersion
	buf[OffsetType] = msgType
	binary.BigEndian.PutUint16(buf[OffsetTag:OffsetTag+TagSize], h.Tag)
	binary.BigEndian.PutUint32(buf[OffsetLen:OffsetLen+LenSize], payloadLen)
	return buf
}

// Message is the interface for all protocol messages.
type Payload interface {
	Encode() []byte
	Type() uint8
	Size() uint32
}

// InboundRequest marks inbound messages handled as requests.
type InboundRequest interface {
	Payload
	inboundRequest()
}

// OkMessage is a zero-payload acknowledgment sent after a message
// has been successfully handled.
type Ok struct{}

func (Ok) Encode() []byte {
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
const HeaderSize = MagicSize + VersionSize + TypeSize + TagSize + LenSize

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

	copy(header.Magic[:], headerBuf[OffsetMagic:OffsetVersion])
	header.Version = headerBuf[OffsetVersion]
	header.Type = headerBuf[OffsetType]
	header.Tag = binary.BigEndian.Uint16(headerBuf[OffsetTag : OffsetTag+TagSize])
	header.PayloadLength = binary.BigEndian.Uint32(headerBuf[OffsetLen : OffsetLen+LenSize])

	if header.Magic != [4]byte{0x4C, 0x4E, 0x4E, 0x00} {
		return header, errors.New("invalid magic")
	}

	if header.Version != 0x01 {
		return header, errors.New("unsupported version")
	}

	return header, nil
}
