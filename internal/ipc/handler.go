// Package ipc handles inter-process communication with the daemon.
package ipc

import (
	"errors"
	"log"
	"net"
	"path"
	"strings"
)

// Validate checks and normalizes an IPC request.
// Parses the IP address from the Src field and validates format.
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

// HandleRequest processes an incoming IPC request based on operation type.
func HandleRequest(req *Request) {
	switch req.Op {
	case "link":
		if err := Validate(req); err != nil {
			log.Printf("Validation error: %v", err)
			return
		}
		switch req.Type {
		case "sym":
			log.Printf("Type: %v.\n", req.Type)
		case "hard":
			log.Printf("Type: %v.\n", req.Type)
		default:
			log.Fatalf("Unknown type: %v", req.Type)
		}
	default:
		log.Fatalf("Unknown operation: %v", req.Op)
	}
}
