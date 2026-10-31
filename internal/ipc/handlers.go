// Package ipc handles inter-process communication with the daemon.
package ipc

import (
	"errors"
	"fmt"
	app "lnn/internal"
	"lnn/internal/config"
	"lnn/internal/network"
	"lnn/internal/protocol"
	"log"
	"net"
	"path"
	"strings"
)

// Validate checks and normalizes an IPC request.
// Expects Src in "IP://path" format; extracts the IP and cleans the path component.
func Validate(req *Request) error {
	if req.Src == "" {
		return errors.New("you must include a source path")
	}
	if !strings.Contains(req.Src, "://") {
		return errors.New("expected IP")
	}
	parts := strings.SplitN(req.Src, "://", 2)
	cleanPath := path.Clean("/" + parts[1])

	ip := net.ParseIP(parts[0])
	if ip == nil {
		return errors.New("invalid IP address")
	}
	req.IP = parts[0]

	req.Src = cleanPath
	return nil
}

// HandleRequest dispatches an incoming IPC request to the appropriate handler.
func HandleRequest(req *Request, unixConn net.Conn, s app.Services) {
	switch req.Op {
	case "link":
		if err := Validate(req); err != nil {
			log.Printf("validation error: %v", err)
			return
		}
		switch req.Type {
		case "symreq":
			transport, err := network.NewTransport(config.DefaultNetworkConfig())
			if err != nil {
				bytes, err := Response{
					Ok:  false,
					Err: err.Error(),
				}.Encode()
				if err != nil {
					return
				}
				_, err = unixConn.Write(bytes)
				if err != nil {
					return
				}
				return
			}
			conn, err := transport.Dial()
			if err != nil {
				bytes, err := Response{
					Ok:  false,
					Err: err.Error(),
				}.Encode()
				if err != nil {
					return
				}
				_, err = unixConn.Write(bytes)
				if err != nil {
					return
				}
				return	
			}
			session, err := network.OutboundHandshake(conn, s.Identity, s.Session)
			if err != nil {
				bytes, err := Response{
					Ok:  false,
					Err: err.Error(),
				}.Encode()
				if err != nil {
					return
				}
				_, err = unixConn.Write(bytes)
				if err != nil {
					return
				}
				return
			}

			fmt.Printf("%v", req.Src)
			err = network.SendTCP(session, protocol.Frame{
				Body: protocol.SymLinkRequest{
					Src: req.Src,
					Dst: req.Dst,
				},
			})
			if err != nil {
				return
			}

			bytes, err := Response{Ok: true, Err: ""}.Encode()
			if err != nil {
				return
			}
			_, err = unixConn.Write(bytes)
			if err != nil {
				return
			}
		case "hard":
			log.Printf("Type: %v.\n", req.Type)
		default:
			log.Printf("unknown type: %v", req.Type)
		}
	default:
		log.Printf("unknown operation: %v", req.Op)
	}
}
