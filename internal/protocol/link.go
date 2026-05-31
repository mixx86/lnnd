package protocol

import (
	"encoding/binary"
	"errors"
	"path/filepath"
)

// SymLinkRequest is sent by a peer requesting this node to create a symlink.
// Dst is optional: if empty, the receiver decides the local path on --accept.
// If Dst is set, the receiver cannot change it — only accept or reject.
type SymLinkRequest struct {
	Src string
	Dst string
}

// Payload layout: [src_len:2][src][dst_len:2][dst]
// Variable-length rather than fixed 8 KB to avoid padding on every request.
func EncodeSymLinkRequest(src string, dst string) []byte {
	payloadSize := 2 + len(src)
	if dst != "" {
		payloadSize += 2 + len(dst)
	}
	packet := make([]byte, payloadSize)

	cursor := 0
	binary.BigEndian.PutUint16(packet[:], uint16(len(src)))
	cursor += 2
	copy(packet[cursor:], src)
	if dst != "" {
		cursor += len(src)
		binary.BigEndian.PutUint16(packet[cursor:], uint16(len(dst)))
		cursor += 2
		copy(packet[cursor:], dst)
	}

	return packet
}

// Encode serializes the symlink request into wire format.
func (r SymLinkRequest) Encode() []byte {
	return EncodeSymLinkRequest(r.Src, r.Dst)
}

func (SymLinkRequest) Type() uint8 { return MsgSymLinkRequest }
func (r SymLinkRequest) Size() uint32 {
	var payloadSize uint32
	if r.Src != "" {
		payloadSize += uint32(2 + len(r.Src))
	}

	if r.Dst != "" {
		payloadSize += uint32(2 + len(r.Dst))
	}

	return payloadSize
}

// ParseSymLinkRequest decodes a symlink request from its binary wire format.
func ParseSymLinkRequest(payload []byte) (SymLinkRequest, error) {
	lenSrc := binary.BigEndian.Uint16(payload[:2])

	if len(payload) < int(2+lenSrc) {
		return SymLinkRequest{}, errors.New("payload too short for src")
	}

	// 4096: matches Linux PATH_MAX; also caps memory from malformed payloads.
	if lenSrc > 4096 {
		return SymLinkRequest{}, errors.New("path too long for src")
	}
	srcStart := 2
	srcEnd := srcStart + int(lenSrc)

	var Dst []byte
	remaining := len(payload) - srcEnd
	switch {
	case remaining == 0:
		// dst omitted — receiver decides the path on --accept.

	case remaining <= 2:
		// dst_len present but no dst bytes: malformed.
		return SymLinkRequest{}, errors.New("incongruent data: dst_len present but dst missing")

	default:
		lenDst := binary.BigEndian.Uint16(payload[srcEnd : srcEnd+2])

		dstStart := srcEnd + 2
		dstEnd := dstStart + int(lenDst)

		if len(payload) < dstEnd {
			return SymLinkRequest{}, errors.New("payload too short for dst")
		}

		Dst = payload[dstStart:dstEnd]
	}

	srcStr := string(payload[srcStart:srcEnd])
	dstStr := ""
	if len(Dst) > 0 {
		dstStr = string(Dst)
	}

	if !filepath.IsAbs(srcStr) {
		return SymLinkRequest{}, errors.New("relative paths not allowed for src")
	}

	cleanSrc := filepath.Clean(srcStr)
	cleanDst := ""
	if dstStr != "" {
		if !filepath.IsAbs(dstStr) {
			return SymLinkRequest{}, errors.New("relative paths not allowed for dst")
		}
		cleanDst = filepath.Clean(dstStr)
	}

	return SymLinkRequest{
		Src: cleanSrc,
		Dst: cleanDst,
	}, nil
}

func (SymLinkRequest) inboundRequest() {}
