package protocol

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
)

// Every request payload starts with sender_req_id, the sender's id for it,
// whatever the request's type, so the requests WAL can keep it in a field of
// its own.
const SenderReqIDLen = 4

// Dst is optional: if empty, the receiver decides the local path.
type SymlinkRequest struct {
	SenderReqID uint32
	Src         string
	Dst         string
}

// Payload layout: [sender_req_id:4][src_len:2][src][dst_len:2][dst]
// Variable-length rather than fixed 8 KB to avoid padding on every request.
func EncodeSymlinkRequest(senderReqID uint32, src string, dst string) []byte {
	payloadSize := SenderReqIDLen + 2 + len(src)
	if dst != "" {
		payloadSize += 2 + len(dst)
	}
	packet := make([]byte, payloadSize)

	binary.BigEndian.PutUint32(packet[:SenderReqIDLen], senderReqID)
	cursor := SenderReqIDLen
	binary.BigEndian.PutUint16(packet[cursor:], uint16(len(src)))
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
	return EncodeSymlinkRequest(r.SenderReqID, r.Src, r.Dst)
}

func (SymlinkRequest) Type() MsgType { return MsgSymlinkRequest }

func (r SymlinkRequest) Size() uint32 {
	payloadSize := uint32(SenderReqIDLen)
	if r.Src != "" {
		payloadSize += uint32(2 + len(r.Src))
	}

	if r.Dst != "" {
		payloadSize += uint32(2 + len(r.Dst))
	}

	return payloadSize
}

func ParseSymlinkRequest(payload []byte) (SymlinkRequest, error) {
	if len(payload) < SenderReqIDLen+2 {
		return SymlinkRequest{}, fmt.Errorf(
			"%w: payload too short for sender_req_id and src_len",
			MsgErrInvalidMessage,
		)
	}
	senderReqID := binary.BigEndian.Uint32(payload[:SenderReqIDLen])
	// Ids start at 1, and 0 is the requests WAL's "no remote id": a request
	// stored with it could not be named in its RESOLUTION.
	if senderReqID == 0 {
		return SymlinkRequest{}, fmt.Errorf(
			"%w: sender_req_id 0",
			MsgErrInvalidMessage,
		)
	}
	payload = payload[SenderReqIDLen:]

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
		SenderReqID: senderReqID,
		Src:         cleanSrc,
		Dst:         cleanDst,
	}, nil
}

func (SymlinkRequest) inboundRequest() {}

const (
	resolutionSenderReqIDLen = 4
	resolutionAcceptedLen    = 1
	ResolutionPayloadSize    = resolutionSenderReqIDLen + resolutionAcceptedLen
)

type Resolution struct {
	LCLReqID uint32
	RMTReqID uint32
	Accepted bool
}

// Payload layout: [sender_req_id:4][accepted:1]
func (r Resolution) Encode() []byte {
	buf := make([]byte, ResolutionPayloadSize)
	binary.BigEndian.PutUint32(buf[:resolutionSenderReqIDLen], r.RMTReqID)
	if r.Accepted {
		buf[resolutionSenderReqIDLen] = 1
	}
	return buf
}

func (Resolution) Type() MsgType { return MsgResolution }
func (Resolution) Size() uint32  { return ResolutionPayloadSize }

func (Resolution) inboundRequest() {}
