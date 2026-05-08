package protocol

import "encoding/binary"

// SymLinkRequest is sent by a peer requesting this node to create a symlink.
// Dst is optional: if empty, the receiver decides the local path on --accept.
// If Dst is set, the receiver cannot change it — only accept or reject.
type SymLinkRequest struct {
	Src string // Path on the sender's machine that the symlink will point to.
	Dst string // Desired path on the receiver's machine; empty if sender leaves it open.
}

// EncodeSymLinkRequest constructs a complete symlink request packet.
// Payload layout: [src_len:2][src:src_len][dst_len:2][dst:dst_len]
func EncodeSymLinkRequest(src string, dst string) []byte {
	// Payload is variable-length (2 B length prefix + path bytes, per field) rather than
	// fixed at the 8194 B maximum, to avoid sending up to 8 KB of padding on every request.
	payloadSize := 2 + len(src) + 2 + len(dst)
	packet := make([]byte, HeaderSize+payloadSize)

	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgLinkSymRequest
	binary.BigEndian.PutUint32(packet[OffsetLen:OffsetBody], uint32(payloadSize))

	binary.BigEndian.PutUint16(packet[OffsetBody:], uint16(len(src)))
	copy(packet[OffsetBody+2:], src)
	binary.BigEndian.PutUint16(packet[OffsetBody+2+len(src):], uint16(len(dst)))
	copy(packet[OffsetBody+2+len(src)+2:], dst)

	return packet
}

// Encode serializes the symlink request into wire format.
func (r SymLinkRequest) Encode() []byte {
	return EncodeSymLinkRequest(r.Src, r.Dst)
}
