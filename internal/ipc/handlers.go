package ipc

import (
	"context"
	"errors"
	"io"
	app "lnn/internal"
	"lnn/internal/network"
	"lnn/internal/protocol"
	"log"
	"net"
	"path"
	"strings"

	"github.com/google/uuid"
)

// ValidateRequest mutates req: IP is split out of Src and Src keeps only the
// cleaned absolute path, so callers must not validate twice.
func ValidateRequest(req *Request, id *uuid.UUID) error {
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

	if req.UUID != "" {
		parsedID, err := uuid.Parse(req.UUID)
		if err != nil {
			log.Printf("identification error: %v", err)
			return err
		}
		*id = parsedID
	}

	return nil
}

func HandleConn(conn net.Conn, s app.Services, srv *network.Server) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing IPC connection: %v", err)
		}
	}()
	data, err := io.ReadAll(conn)
	if err != nil {
		log.Printf("Error receiving data from the peer: %v", err)
		return
	}
	req, err := ParseRequest(data)
	if err != nil {
		log.Printf("Error deserializing json data: %v", err)
		return
	}
	HandleRequest(&req, conn, s, srv)
}

func HandleRequest(
	req *Request,
	unixConn net.Conn,
	s app.Services,
	srv *network.Server,
) {
	switch req.Op {
	case "link":
		id := uuid.UUID{}
		err := ValidateRequest(req, &id)
		if err != nil {
			log.Printf("validation error: %v", err)
			Send(unixConn, Response{
				OK:  false,
				Err: err.Error(),
			})
			return
		}

		switch req.Type {
		case "symreq":
			session, exists := s.Session.Get(id)

			if exists {
				frame := s.Outbox.Put(
					session.Remote.ID,
					protocol.SymlinkRequest{
						Src: req.Src,
						Dst: req.Dst,
					},
				)
				if err := network.SendFrame(session, frame); err != nil {
					Send(unixConn, Response{
						OK:  false,
						Err: err.Error(),
					})
					return
				}
				Send(unixConn, Response{
					OK:  true,
					Err: "",
				})
				return
			}

			session, err = srv.Connect(context.Background(), req.IP)
			if err != nil {
				Send(unixConn, Response{
					OK:  false,
					Err: err.Error(),
				})
				return
			}

			frame := s.Outbox.Put(session.Remote.ID, protocol.SymlinkRequest{
				Src: req.Src,
				Dst: req.Dst,
			})

			if err := network.SendFrame(session, frame); err != nil {
				Send(unixConn, Response{
					OK:  false,
					Err: err.Error(),
				})
				return
			}

			Send(unixConn, Response{
				OK:  true,
				Err: "",
			})

			return
		case "hard":
			log.Printf("Type: %v.\n", req.Type)
		default:
			log.Printf("unknown type: %v", req.Type)
		}
	default:
		log.Printf("unknown operation: %v", req.Op)
	}
}
