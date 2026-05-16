package network

import (
	"errors"
	"fmt"
	"io"
	"lnn/internal/identity"
	"lnn/internal/logger"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
	"os"
)

// ConnectTCP establishes a TCP connection to the given IP address.
// Uses TCP_PORT environment variable for the port.
func ConnectTCP(ip string) (net.Conn, error) {
	port := os.Getenv("TCP_PORT")
	return net.Dial("tcp", ip+":"+port)
}

// ListenTCP binds to TCP_PORT and accepts incoming peer connections indefinitely.
// Returns only if the bind fails; a successful call never returns.
func ListenTCP(i *identity.IdentityService, pm *peer.PeerManager) error {
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
		go HandleConnTCP(conn, i, pm)
	}
}

// SendTCP encodes and sends a protocol message to a peer.
// Safe to call concurrently from multiple goroutines.
func SendTCP(p *peer.Peer, data protocol.Message) error {
	packet := data.Encode()

	// Since either IPC or TCP server can send packets, we need to lock with mutex before the write operation to avoid collisions.
	p.Mu.Lock()
	defer p.Mu.Unlock()
	_, err := p.Conn.Write(packet)
	return err
}

// In order to send packets when the peer is not initialized, we need to write without locking.
// This is actually not an issue since no other service than handshake is allowed to send messages while the auth process is happening.
func sendRaw(conn net.Conn, data protocol.Message) error {
	packet := data.Encode()

	_, err := conn.Write(packet)

	return err
}

// ReceiveTCP reads a complete protocol message from conn and returns it as the
// appropriate concrete type. Blocks until a full message arrives or an error occurs.
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
