package client

import (
	"fmt"
	"io"
	"lnn/internal/protocol"
	"log"
	"os"

	"net"

	"github.com/google/uuid"
)

func ConnectTCP(ip string) (net.Conn, error) {
	port := os.Getenv("TCP_PORT")
	return net.Dial("tcp", ip+":"+port)
}

func SendTCP(conn net.Conn, data protocol.Message) error {
	packet, err := data.Encode()
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	n, err := conn.Write(packet)
	if err != nil {
		return err
	}
	log.Printf("Sent %v bytes", n)
	return err
}

func RecvTCP(conn net.Conn) (any, error) {
	header, err := protocol.ReadHeader(conn)
	if err != nil {
		return nil, err
	}
	switch header.Type {
	case protocol.MsgHello:
		id := make([]byte, protocol.HelloPayloadSize)
		_, err := io.ReadFull(conn, id)
		if err != nil {
			return uuid.Nil, err
		}
		return protocol.ParseHello(id)
	default:
		return nil, fmt.Errorf("unknown message type: %d", header.Type)
	}
}
