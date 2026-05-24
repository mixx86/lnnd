package network

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"lnn/internal/identity"
	"lnn/internal/iocore"
	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/protocol"
)

// ConnectTCP establishes a TCP connection to the given IP address.
func ConnectTCP(ip string) (net.Conn, error) {
	port := os.Getenv("TCP_PORT")
	return net.Dial("tcp", ip+":"+port)
}

// ListenTCP binds to TCP_PORT and accepts incoming peer connections indefinitely.
func ListenTCP(i *identity.IdentityService, sm *peer.SessionManager, ioc *iocore.Manager) error {
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
		go HandleConnTCP(conn, i, sm, ioc)
	}
}

// SendTCP encodes and sends a protocol message to a peer.
func SendTCP(s *peer.Session, data protocol.Message) error {
	packet := data.Encode()

	// Both the TCP read loop and the IPC handler can send; lock to avoid interleaving.
	s.Mu.Lock()
	defer s.Mu.Unlock()
	_, err := s.Conn.Write(packet)
	return err
}

// Pre-handshake only: no Peer exists yet and the conn is owned by a single goroutine, so no lock needed.
func sendRaw(conn net.Conn, data protocol.Message) error {
	packet := data.Encode()

	_, err := conn.Write(packet)

	return err
}

// ReceiveTCP reads a complete protocol message from conn and returns it as the appropriate concrete type.
func ReceiveTCP(conn net.Conn) (protocol.Message, error) {
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		return nil, err
	}

	switch header.Type {
	case protocol.MsgHello:
		payload := make([]byte, protocol.HelloPayloadSize)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.HelloMessage{}, err
		}

		return protocol.ParseHello(payload)

	case protocol.MsgChallenge:
		payload := make([]byte, protocol.ChallengePayloadSize)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.ChallengeMessage{}, err
		}

		return protocol.ParseChallenge(payload)

	case protocol.MsgChallengeResp:
		payload := make([]byte, protocol.ChallengeRespPayloadSize)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.ChallengeRespMessage{}, err
		}

		return protocol.ParseChallengeResp(payload)

	case protocol.MsgSymLinkRequest:
		if header.PayloadLength > 8192 || header.PayloadLength < 3 {
			return protocol.SymLinkRequest{}, errors.New("invalid payload size")
		}
		payload := make([]byte, header.PayloadLength)
		_, err := io.ReadFull(conn, payload)

		if err != nil {
			return protocol.SymLinkRequest{}, err
		}

		return protocol.ParseSymLinkRequest(payload)

	default:
		return nil, fmt.Errorf("unknown message type: %d", header.Type)
	}
}
