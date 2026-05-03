package network

import (
	"fmt"
	"io"
	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"log"
	"net"
	"os"
)

// ConnectTCP establishes a TCP connection to the given IP address.
// Uses TCP_PORT environment variable for the port.
func ConnectTCP(ip string) (net.Conn, error) {
	port := os.Getenv("TCP_PORT")
	return net.Dial("tcp", ip+":"+port)
}

// ListenTCP starts the TCP server and handles incoming connections.
// Uses TCP_PORT environment variable for the port.
// This function blocks indefinitely.
func ListenTCP(i *identity.IdentityService, pm *peer.PeerManager) {
	port := os.Getenv("TCP_PORT")
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Error opening the connection: %v", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalf("Error accepting the connection: %v", err)
		}
		go HandleConnTCP(conn, i, pm)
	}
}

// SendTCP encodes and sends a protocol message over the connection.
func SendTCP(conn net.Conn, data protocol.Message) error {
	packet, err := data.Encode()
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	_, err = conn.Write(packet)

	return err
}

// ReceiveTCP reads and parses a protocol message from the connection.
// Returns the appropriate message type based on the header.
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

	default:
		return nil, fmt.Errorf("unknown message type: %d", header.Type)
	}

}
