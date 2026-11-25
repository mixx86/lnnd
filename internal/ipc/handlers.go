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

func ValidateRequest(req *Request) error {
	if req.IP == "" {
		return errors.New("you must include the peer's IP")
	}
	if net.ParseIP(req.IP) == nil {
		return errors.New("invalid IP address")
	}
	return nil
}

// ValidateLinkRequest mutates req: Src is cleaned and made absolute.
func ValidateLinkRequest(req *LinkRequest) error {
	if err := ValidateRequest(&req.Request); err != nil {
		return err
	}
	if req.Src == "" {
		return errors.New("you must include a source path")
	}
	req.Src = path.Clean("/" + req.Src)
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
	case *LinkRequest:
		if err := ValidateLinkRequest(m); err != nil {
			log.Printf("validation error: %v", err)
			SendErr(conn, ErrInvalidRequest, err)
			return
		}
		HandleLinkRequest(m, id, conn, s, srv)
	case *Request:
		HandleRequest(m, conn)
	}
}

// A send reaches HandleRequest only when ParseMessage found no struct for
// its kind.
func HandleRequest(req *Request, conn net.Conn) {
	switch req.Op {
	case "send":
		log.Printf("unknown kind: %v", req.Kind)
		SendErr(
			conn,
			ErrInvalidRequest,
			fmt.Errorf("unknown kind: %v", req.Kind),
		)
	case "accept":
		log.Printf("TODO: accept request")
	case "reject":
		log.Printf("TODO: reject request")
	case "list":
		log.Printf("TODO: list requests")
	default:
		log.Printf("unknown operation: %v", req.Op)
		SendErr(
			conn,
			ErrUnknownOp,
			fmt.Errorf("unknown operation: %v", req.Op),
		)
	}
}

func HandleLinkRequest(
	req *LinkRequest,
	id uuid.UUID,
	conn net.Conn,
	s app.Services,
	srv *network.Server,
) {
	switch req.Kind {
	case "symlink":
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
		link := protocol.SymlinkRequest{Src: req.Src, Dst: req.Dst}
		// Logged before it is sent: once the peer may hold the request,
		// we must hold its record too.
		link.SenderReqID, err = srv.Scheduler.SubmitSentRequest(
			sess.Remote,
			link,
		)
		if err != nil {
			log.Printf("Error logging sent request: %v", err)
			SendErr(conn, ErrInternal, err)
			return
		}
		reply, err := srv.RoundTrip(ctx, sess, link)

		if err != nil {
			SendErr(conn, ErrNoReply, err)
			return
		}
		if e, ok := reply.(protocol.Error); ok {
			SendErr(conn, ErrPeer, e)
			return
		}
		SendOk(conn)
	case "hardlink":
		log.Printf("Kind: %v.\n", req.Kind)
	}
}
