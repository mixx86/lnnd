package protocol

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
)

// Dst is optional: if empty, the receiver decides the local path.
type SymlinkRequest struct {
	Src string
	Dst string
}

// Payload layout: [src_len:2][src][dst_len:2][dst]
// Variable-length rather than fixed 8 KB to avoid padding on every request.
func EncodeSymlinkRequest(src string, dst string) []byte {
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

func (r SymlinkRequest) Encode() []byte {
	return EncodeSymlinkRequest(r.Src, r.Dst)
}

func (SymlinkRequest) Type() MsgType { return MsgSymlinkRequest }

func (r SymlinkRequest) Size() uint32 {
	var payloadSize uint32
	if r.Src != "" {
		payloadSize += uint32(2 + len(r.Src))
	}

	if r.Dst != "" {
		payloadSize += uint32(2 + len(r.Dst))
	}

	return payloadSize
}

func ParseSymlinkRequest(payload []byte) (SymlinkRequest, error) {
	lenSrc := binary.BigEndian.Uint16(payload[:2])

	if len(payload) < int(2+lenSrc) {
		return SymlinkRequest{}, fmt.Errorf(
			"%w: payload too short for src",
			MsgErrInvalidMessage,
		)
	}

	// 4096: matches Linux PATH_MAX; also caps memory from malformed payloads.
	if lenSrc > 4096 {
		return SymlinkRequest{}, fmt.Errorf(
			"%w: path too long for src",
			MsgErrInvalidMessage,
		)
	}
	srcStart := 2
	srcEnd := srcStart + int(lenSrc)

	var Dst []byte
	remaining := len(payload) - srcEnd
	switch {
	case remaining == 0:
		// dst omitted

	case remaining <= 2:
		return SymlinkRequest{}, fmt.Errorf(
			"%w: incongruent data: dst_len present but dst missing",
			MsgErrInvalidMessage,
		)

	default:
		lenDst := binary.BigEndian.Uint16(payload[srcEnd : srcEnd+2])

		dstStart := srcEnd + 2
		dstEnd := dstStart + int(lenDst)

		if len(payload) < dstEnd {
			return SymlinkRequest{}, fmt.Errorf(
				"%w: payload too short for dst",
				MsgErrInvalidMessage,
			)
		}

		Dst = payload[dstStart:dstEnd]
	}

	srcStr := string(payload[srcStart:srcEnd])
	dstStr := ""
	if len(Dst) > 0 {
		dstStr = string(Dst)
	}

	if !filepath.IsAbs(srcStr) {
		return SymlinkRequest{}, fmt.Errorf(
			"%w: relative paths not allowed for src",
			MsgErrInvalidMessage,
		)
	}

	cleanSrc := filepath.Clean(srcStr)
	cleanDst := ""
	if dstStr != "" {
		if !filepath.IsAbs(dstStr) {
			return SymlinkRequest{}, fmt.Errorf(
				"%w: relative paths not allowed for dst",
				MsgErrInvalidMessage,
			)
		}
		cleanDst = filepath.Clean(dstStr)
	}

	return SymlinkRequest{
		Src: cleanSrc,
		Dst: cleanDst,
	}, nil
}

func (SymlinkRequest) inboundRequest() {}

const (
	resolutionReqIDLen    = 4
	resolutionAcceptedLen = 1
	ResolutionPayloadSize = resolutionReqIDLen + resolutionAcceptedLen
)

type Resolution struct {
	LCLReqID uint32
	RMTReqID uint32
	Accepted bool
}

// Payload layout: [rmt_req_id:4][accepted:1]
func (r Resolution) Encode() []byte {
	buf := make([]byte, ResolutionPayloadSize)
	binary.BigEndian.PutUint32(buf[:resolutionReqIDLen], r.RMTReqID)
	if r.Accepted {
		buf[resolutionReqIDLen] = 1
	}
	return buf
}

func (Resolution) Type() MsgType { return MsgResolution }
func (Resolution) Size() uint32  { return ResolutionPayloadSize }

func (Resolution) inboundRequest() {}
