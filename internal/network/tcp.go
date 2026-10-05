package network

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"lnn/internal/identity"
	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/sched"
)

func ConnectTCP(ip string) (net.Conn, error) {
	port := os.Getenv("TCP_PORT")
	return net.Dial("tcp", ip+":"+port)
}

func ListenTCP(sm *peer.SessionManager, schedService *sched.Service, idService *identity.Service) error {
	port := os.Getenv("TCP_PORT")
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Daemon.Printf("Error accepting TCP connection: %v", err)
			continue
		}
		go HandleConnTCP(conn, sm, schedService, idService)
	}
}

func SendTCP(s *peer.Session, data protocol.Frame) error {
	packet := data.Encode()

	// Both the TCP read loop and the IPC handler can send; lock to avoid interleaving.
	s.Mu.Lock()
	defer s.Mu.Unlock()
	_, err := s.Conn.Write(packet)
	return err
}

// Pre-handshake only: no Peer exists yet and the conn is owned by a single goroutine, so no lock needed.
func sendRaw(conn net.Conn, data protocol.Frame) error {
	packet := data.Encode()

	_, err := conn.Write(packet)

	return err
}

func ReceiveTCP(conn net.Conn) (protocol.Frame, error) {
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		return protocol.Frame{}, err
	}

	switch header.Type {
	case protocol.MsgHello:
		payload := make([]byte, protocol.HelloPayloadSize)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.Frame{}, err
		}

		hello, err := protocol.ParseHello(payload)
		if err != nil {
			return protocol.Frame{}, err
		}

		return protocol.Frame{
			Header: header,
			Body:   hello,
		}, nil

	case protocol.MsgChallenge:
		payload := make([]byte, protocol.ChallengePayloadSize)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.Frame{}, err
		}

		challenge, err := protocol.ParseChallenge(payload)
		if err != nil {
			return protocol.Frame{}, err
		}

		return protocol.Frame{
			Header: header,
			Body:   challenge,
		}, nil

	case protocol.MsgChallengeResp:
		payload := make([]byte, protocol.ChallengeRespPayloadSize)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.Frame{}, err
		}

		challenge, err := protocol.ParseChallengeResp(payload)
		if err != nil {
			return protocol.Frame{}, err
		}
		return protocol.Frame{
			Header: header,
			Body:   challenge,
		}, nil

	case protocol.MsgSymLinkRequest:
		if header.PayloadLength > 8192 || header.PayloadLength < 3 {
			return protocol.Frame{}, errors.New("invalid payload size")
		}
		payload := make([]byte, header.PayloadLength)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.Frame{}, err
		}

		symlinkreq, err := protocol.ParseSymLinkRequest(payload)
		if err != nil {
			return protocol.Frame{}, err
		}

		return protocol.Frame{
			Header: header,
			Body:   symlinkreq,
		}, nil

	default:
		return protocol.Frame{}, fmt.Errorf("unknown message type: %d", header.Type)
	}
}
