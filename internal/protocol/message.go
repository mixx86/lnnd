package protocol

import (
	"encoding/binary"

	"github.com/google/uuid"
)

const (
	MsgHello uint8 = 0
	MsgError uint8 = 1
)

func BuildHello(id uuid.UUID) []byte {
	header := []byte{0x4C, 0x4E, 0x4E, 0x00, 0x01}
	header = append(header, MsgHello)
	header = binary.BigEndian.AppendUint32(header, uint32(len(id.String())))
	header = append(header, []byte(id.String())...)
	return header
}
