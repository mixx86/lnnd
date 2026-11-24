package ipc

import (
	"context"
	"errors"
	"fmt"
	"io"
	app "lnn/internal"
	"lnn/internal/network"
	"lnn/internal/protocol"
	"log"
	"net"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxMessageSize = 100 << 10

func ValidateMessage(h *Header, id *uuid.UUID) error {
	if h.Op == "" {
		return errors.New("you must include an operation")
	}
	if h.UUID != "" {
		parsedID, err := uuid.Parse(h.UUID)
		if err != nil {
			log.Printf("identification error: %v", err)
			return err
		}
		*id = parsedID
	}
	return nil
}

// ValidateRequest mutates req: IP is split out of Src and Src keeps only the
// cleaned absolute path, so callers must not validate twice.
func ValidateRequest(req *Request) error {
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

func HandleConn(conn net.Conn, s app.Services, srv *network.Server) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing IPC connection: %v", err)
		}
	}()
	err := conn.SetReadDeadline(time.Now().Add(time.Second * 2))
	if err != nil {
		log.Printf("Error setting up the IPC connection deadline: %v", err)
		return
	}
	data, err := io.ReadAll(io.LimitReader(conn, maxMessageSize+1))
	if len(data) > maxMessageSize {
		log.Printf("Error setting IPC read deadline: %v", err)
		SendErr(conn, ErrMessageTooLarge, fmt.Errorf("message too large: limit is %d bytes", maxMessageSize))
		return
	}
	if err != nil {
		log.Printf("Error receiving data from the client: %v", err)
		SendErr(conn, ErrTimeout, err)
		return
	}

	msg, err := ParseMessage(data)
	if errors.Is(err, errUnknownType) {
		log.Printf("Error parsing message: %v", err)
		SendErr(conn, ErrUnknownType, err)
		return
	}
	if err != nil {
		log.Printf("Error deserializing json data: %v", err)
		SendErr(conn, ErrInvalidMessage, err)
		return
	}
	id := uuid.UUID{}
	if err := ValidateMessage(msg.header(), &id); err != nil {
		log.Printf("validation error: %v", err)
		SendErr(conn, ErrInvalidMessage, err)
		return
	}
	HandleMessage(msg, id, conn, s, srv)
}

func HandleMessage(
	msg Message,
	id uuid.UUID,
	conn net.Conn,
	s app.Services,
	srv *network.Server,
) {
	switch m := msg.(type) {
	case *Request:
		if err := ValidateRequest(m); err != nil {
			log.Printf("validation error: %v", err)
			SendErr(conn, ErrInvalidRequest, err)
			return
		}
		HandleRequest(m, id, conn, s, srv)
	}
}

func HandleRequest(
	req *Request,
	id uuid.UUID,
	conn net.Conn,
	s app.Services,
	srv *network.Server,
) {
	switch req.Op {
	case "link":
		switch req.Link {
		case "sym":
			ctx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()

			sess, err := srv.SessionFor(ctx, id, req.IP)
			if err != nil {
				SendErr(conn, ErrPeerUnreachable, err)
				return
			}
			reply, err := srv.RoundTrip(ctx, sess, protocol.SymlinkRequest{
				Src: req.Src,
				Dst: req.Dst,
			})

			if err != nil {
				SendErr(conn, ErrNoReply, err)
				return
			}
			if e, ok := reply.(protocol.Error); ok {
				SendErr(conn, ErrPeer, e)
				return
			}
			SendOk(conn)

			return
		case "hard":
			log.Printf("Link: %v.\n", req.Link)
		default:
			log.Printf("unknown link: %v", req.Link)
			SendErr(
				conn,
				ErrInvalidRequest,
				fmt.Errorf("unknown link: %v", req.Link),
			)
		}
	default:
		log.Printf("unknown operation: %v", req.Op)
		SendErr(conn, ErrUnknownOp, fmt.Errorf("unknown operation: %v", req.Op))
	}
}
