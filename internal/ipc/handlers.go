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
// Expects Src in "IP://path" format; extracts the IP into req.IP and
// cleans the path component. Returns an error if the format is invalid
// or the IP address cannot be parsed.
func Validate(req *Request) error {
	if req.Src == "" {
		return errors.New("you must include a source path")
	}
	if !strings.Contains(req.Src, "://") {
		return errors.New("expected IP")
	}
	parts := strings.SplitN(req.Src, "://", 2)
	cleanPath := path.Clean(parts[1])

	ip := net.ParseIP(parts[0])
	if ip == nil {
		return errors.New("invalid IP address")
	}
	req.IP = parts[0]

	req.Src = parts[0] + "://" + cleanPath
	return nil
}

// HandleRequest dispatches an incoming IPC request to the appropriate handler.
// Validates the request before dispatching; unknown operations and types are logged and dropped.
func HandleRequest(req *Request) {
	switch req.Op {
	case "link":
		if err := Validate(req); err != nil {
			log.Printf("validation error: %v", err)
			return
		}
		switch req.Type {
		case "sym":
			log.Printf("Type: %v.\n", req.Type)
		case "hard":
			log.Printf("Type: %v.\n", req.Type)
		default:
			log.Printf("unknown type: %v", req.Type)
		}
	default:
		log.Printf("unknown operation: %v", req.Op)
	}
}
