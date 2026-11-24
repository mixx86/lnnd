package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
)

type Code int64

const (
	OK = iota
	ErrInvalidMessage
	ErrMessageTooLarge
	ErrInvalidRequest
	ErrUnknownOp
	ErrUnknownType
	ErrPeerUnreachable
	ErrNoReply
	ErrPeer
	ErrInternal
	ErrTimeout
)

type Response struct {
	Code Code   `json:"code"`
	Err  string `json:"err,omitempty"`
}

func (r Response) Encode() ([]byte, error) {
	return json.Marshal(r)
}

type Header struct {
	Type string `json:"type"`
	Op   string `json:"op"`
	UUID string `json:"uuid"`
}

type Message interface {
	header() *Header
}

type Request struct {
	Header
	Kind string `json:"kind"`
	IP   string `json:"ip"`
}

func (r *Request) header() *Header { return &r.Header }

type LinkRequest struct {
	Request
	Src string `json:"src"`
	Dst string `json:"dst"`
}

var errUnknownType = errors.New("unknown message type")

func ParseMessage(data []byte) (Message, error) {
	var h Header
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	switch h.Type {
	case "request":
		return parseRequest(data)
	default:
		return nil, fmt.Errorf("%w: %q", errUnknownType, h.Type)
	}
}

// Only a send names a kind; any other op, or a kind without a struct of its
// own, comes back as a bare *Request so HandleMessage can say which is
// unknown.
func parseRequest(data []byte) (Message, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	if req.Op != "send" {
		return &req, nil
	}
	switch req.Kind {
	case "symlink", "hardlink":
		var link LinkRequest
		if err := json.Unmarshal(data, &link); err != nil {
			return nil, err
		}
		return &link, nil
	default:
		return &req, nil
	}
}
