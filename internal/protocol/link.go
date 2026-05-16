package protocol

import (
	"encoding/binary"
	"errors"
)

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
	// fixed at the 8196 B maximum, to avoid sending up to 8 KB of padding on every request.
	payloadSize := 2 + len(src) + 2 + len(dst)
	packet := make([]byte, HeaderSize+payloadSize)

	copy(packet[OffsetMagic:OffsetMagic+MagicSize], protocolMagic[:])
	packet[OffsetVersion] = protocolVersion
	packet[OffsetType] = MsgSymLinkRequest
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


// ParseSymLinkRequest decodes a symlink request from its binary wire format.
//
// The payload is structured as:
//   [src_len:2][src][dst_len:2][dst]
//
// The dst field is optional at the protocol level, but only considered valid if:
// - dst_len is present AND
// - at least 1 byte of dst data follows it
//
// Truncated or inconsistent payloads are rejected to avoid silently accepting
// corrupted or partially transmitted requests.
func ParseSymLinkRequest(payload []byte) (SymLinkRequest, error) {
	lenSrc := binary.BigEndian.Uint16(payload[:2])

	// Enforce structural integrity: src_len must match actual available bytes.
	// This prevents slicing beyond the buffer and avoids accepting truncated src data.
	if len(payload) < int(2+lenSrc) {
		return SymLinkRequest{}, errors.New("payload too short for src")
	}

  // Enforce protocol limit for path length.
	// Although Linux paths can exceed typical PATH_MAX behavior in some cases,
	// most userspace APIs and tooling assume a ~4096 byte limit.
	// This also prevents excessive memory allocation from malformed requests.
	if lenSrc > 4096 {
		return SymLinkRequest{}, errors.New("path too long for src")
	}
	srcStart := 2
	srcEnd := srcStart + int(lenSrc)

	var Dst []byte
	remaining := len(payload) - srcEnd
	switch {
		case remaining == 0:
			// No dst section present: this is a valid request where destination is omitted.

		case remaining <= 2:
			// A dst_len is present but there is no actual dst payload following it.
			// This is considered a malformed packet because length metadata exists
			// without corresponding data.
			return SymLinkRequest{}, errors.New("incongruent data: dst_len present but dst missing")

		default:
			lenDst := binary.BigEndian.Uint16(payload[srcEnd : srcEnd+2])

			dstStart := srcEnd + 2
	  	dstEnd := dstStart + int(lenDst)

			// Validate that declared dst length matches remaining payload size.
  		if len(payload) < dstEnd {
	  		return SymLinkRequest{}, errors.New("payload too short for dst")
	  	}

			Dst = payload[dstStart:dstEnd]
	}

	dstStr := ""
	if len(Dst) > 0 {
		dstStr = string(Dst)
	}

	return SymLinkRequest{
		Src: string(payload[srcStart:srcEnd]),
		Dst: dstStr,
	}, nil
}
