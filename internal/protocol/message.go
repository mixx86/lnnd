package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/google/uuid"
)

type Header struct {
	Type   uint8
	Length uint32
}

// type of message by num
const (
	MsgHello uint8 = 0
	MsgError uint8 = 1
)

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

const HelloPayloadSize uint32 = 16

type Message interface {
	Encode() ([]byte, error)
}

type HelloMessage struct {
	ID uuid.UUID
}

func (m HelloMessage) Encode() ([]byte, error) {
	return BuildHello(m.ID), nil
}

var baseHeader = [5]byte{0x4C, 0x4E, 0x4E, 0x00, 0x01}

func BuildHello(id uuid.UUID) []byte {
	packet := make([]byte, HeaderSize+HelloPayloadSize)
	copy(packet[OffsetMagic:OffsetType], baseHeader[:])
	packet[OffsetType] = MsgHello
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], HelloPayloadSize)
	copy(packet[OffsetBody:], id[:])
	return packet
}

func ParseHello(id []byte) (uuid.UUID, error) {
	parsedID, err := uuid.FromBytes(id)
	if err != nil {
		return uuid.Nil, err
	}
	return parsedID, nil
}

var magic = baseHeader[0:5]

func ValidateProtocol(data []byte) error {
	if data[0] != magic[0] ||
		data[1] != magic[1] ||
		data[2] != magic[2] ||
		data[3] != magic[3] {
		return errors.New("invalid magic")
	}

	if data[4] != 0x01 {
		return errors.New("unsupported version")
	}
	return nil
}

func ReadHeader(conn net.Conn) (Header, error) {
	var header Header
	data := make([]byte, HeaderSize)
	_, err := io.ReadFull(conn, data)
	if err != nil {
		return header, err
	}
	err = ValidateProtocol(data)
	if err != nil {
		return header, err
	}
	header.Type = data[5]
	header.Length = binary.BigEndian.Uint32(data[6:10])
	return header, nil
}
