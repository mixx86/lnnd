package ipc

import (
	"errors"
	"lnn/internal/client"
	"lnn/internal/protocol"
	"log"
	"net"
	"path"
	"strings"

	"github.com/google/uuid"
)

func Validate(req *Request) error {
	if req.Src == "" {
		return errors.New("You must include a source path.")
	}
	if !strings.Contains(req.Src, "://") {
		return errors.New("Expected IP")
	}
	parts := strings.SplitN(req.Src, "://", 2)
	cleanPath := path.Clean(parts[1])

	ip := net.ParseIP(parts[0])
	if ip == nil {
		return errors.New("Invalid IP address")
	}
	req.IP = parts[0]

	req.Src = parts[0] + "://" + cleanPath
	return nil
}

func HandleRequest(req *Request, id uuid.UUID) {
	switch req.Op {
	case "link":
		log.Printf("Operation: %v.\n", req.Op)
		if err := Validate(req); err != nil {
			log.Printf("Validation error: %v", err)
			return
		}
		switch req.Type {
		case "sym":
			conn, err := client.ConnectTCP(req.IP)
			if err != nil {
				log.Fatalf("Connection error: %v", err)
			}
			defer conn.Close()
			log.Printf("Request: %v", req)

			remoteID, err := client.RecvTCP(conn)
			if err != nil {
				log.Fatalf("Receiving data error: %v", err)
			}
			log.Printf("uuid received from tcp server: %v", remoteID)

			err = client.SendTCP(conn, protocol.HelloMessage{
				ID: id,
			})
			if err != nil {
				log.Fatalf("Connection error: %v", err)
			}
		case "hard":
			log.Printf("Type: %v.\n", req.Type)
		default:
			log.Fatalf("Unknown type: %v", req.Type)
		}
	default:
		log.Fatalf("Unknown operation: %v", req.Op)
	}

}
