package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

type MsgType uint16

const (
	MsgHello MsgType = iota
	MsgChallenge
	MsgChallengeResp
	MsgError
	// TODO: change name to MsgSymlink
	MsgLinkSym
	MsgLinkHard
	MsgSymlinkRequest
	MsgHardLinkRequest
	MsgOk
	MsgRecoverRequest
	MsgResolution
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

func (m Frame) WithTag(tag uint32) Frame {
	m.Header.Tag = tag
	return m
}

func (h Header) Encode(msgType MsgType, payloadLen uint32) []byte {
	buf := make([]byte, HeaderSize)
	copy(buf[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	buf[OffsetVersion] = protocolVersion
	buf[OffsetType] = byte(msgType)
	binary.BigEndian.PutUint32(buf[OffsetTag:OffsetTag+TagSize], h.Tag)
	binary.BigEndian.PutUint32(buf[OffsetLen:OffsetLen+LenSize], payloadLen)
	return buf
}

type Payload interface {
	Encode() []byte
	Type() MsgType
	Size() uint32
}

type InboundRequest interface {
	Payload
	inboundRequest()
}

type OK struct{}

func (OK) Encode() []byte {
	packet := make([]byte, 0)
	return packet
}

func (OK) Type() MsgType {
	return MsgOk
}

func (OK) Size() uint32 {
	return 0
}

type Header struct {
	Magic         [4]byte
	Version       uint8
	Type          MsgType
	Tag           uint32
	PayloadLength uint32
}

const (
	MagicSize   = 4
	VersionSize = 1
	TypeSize    = 1
	TagSize     = 4
	LenSize     = 4

	OffsetMagic   = 0
	OffsetVersion = 4
	OffsetType    = 5
	OffsetTag     = 6
	OffsetLen     = 10
	OffsetBody    = 14
)
const HeaderSize = MagicSize + VersionSize + TypeSize + TagSize + LenSize

var protocolMagic = [4]byte{0x4C, 0x4E, 0x4E, 0x00}

const protocolVersion uint8 = 0x01

func ReadHeader(r io.Reader) (Header, error) {
	var header Header
	headerBuf := make([]byte, HeaderSize)
	_, err := io.ReadFull(r, headerBuf)
	if err != nil {
		return header, err
	}

	copy(header.Magic[:], headerBuf[OffsetMagic:OffsetVersion])
	header.Version = headerBuf[OffsetVersion]
	header.Type = MsgType(headerBuf[OffsetType])
	header.Tag = binary.BigEndian.Uint32(
		headerBuf[OffsetTag : OffsetTag+TagSize],
	)
	header.PayloadLength = binary.BigEndian.Uint32(
		headerBuf[OffsetLen : OffsetLen+LenSize],
	)

	if header.Magic != [4]byte{0x4C, 0x4E, 0x4E, 0x00} {
		return header, errors.New("invalid magic")
	}

	if header.Version != 0x01 {
		return header, errors.New("unsupported version")
	}

	return header, nil
}
