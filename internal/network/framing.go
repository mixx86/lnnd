package network

import (
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

	n, err := protocol.PayloadLength(header)
	if err != nil {
		return protocol.Frame{}, err
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return protocol.Frame{Header: header}, err
	}

	body, err := protocol.ParsePayload(header.Type, payload)
	if err != nil {
		return protocol.Frame{Header: header}, err
	}

	return protocol.Frame{
		Header: header,
		Body:   body,
	}, nil
}
