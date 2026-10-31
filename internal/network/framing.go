package network

import (
	"errors"
	"fmt"
	"io"
	"lnn/internal/protocol"
)

func WriteFrame(w io.Writer, frame protocol.Frame) error {
	packet := frame.Encode()

	for len(packet) > 0 {
		n, err := w.Write(packet)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}

		packet = packet[n:]
	}

	return nil
}

func ReadFrame(r io.Reader) (protocol.Frame, error) {
	header, err := protocol.ReadHeader(r)
	if err != nil {
		return protocol.Frame{}, err
	}

	var payload []byte

	switch header.Type {
	case protocol.MsgHello:
		payload = make([]byte, protocol.HelloPayloadSize)

	case protocol.MsgChallenge:
		payload = make([]byte, protocol.ChallengePayloadSize)

	case protocol.MsgChallengeResp:
		payload = make([]byte, protocol.ChallengeRespPayloadSize)

	case protocol.MsgSymlinkRequest:
		if header.PayloadLength > 8192 ||
			header.PayloadLength < 3 {
			return protocol.Frame{}, errors.New("invalid payload size")
		}

		payload = make([]byte, header.PayloadLength)

	default:
		return protocol.Frame{}, fmt.Errorf(
			"unknown message type: %d",
			header.Type,
		)
	}

	if _, err := io.ReadFull(r, payload); err != nil {
		return protocol.Frame{}, err
	}

	var body protocol.Payload

	switch header.Type {
	case protocol.MsgHello:
		body, err = protocol.ParseHello(payload)

	case protocol.MsgChallenge:
		body, err = protocol.ParseChallenge(payload)

	case protocol.MsgChallengeResp:
		body, err = protocol.ParseChallengeResp(payload)

	case protocol.MsgSymlinkRequest:
		body, err = protocol.ParseSymlinkRequest(payload)
	}

	if err != nil {
		return protocol.Frame{}, err
	}

	return protocol.Frame{
		Header: header,
		Body:   body,
	}, nil
}
