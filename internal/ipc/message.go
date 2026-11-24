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
	Link string `json:"link"`
	Src  string `json:"src"` // <ip>://<path> until ValidateRequest splits it
	Dst  string `json:"dst"`
	IP   string `json:"-"` // From ValidateRequest; never taken from the client
}

func (r *Request) header() *Header { return &r.Header }

var errUnknownType = errors.New("unknown message type")

func ParseMessage(data []byte) (Message, error) {
	var h Header
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	switch h.Type {
	case "request":
		var req Request
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		return &req, nil
	default:
		return nil, fmt.Errorf("%w: %q", errUnknownType, h.Type)
	}
}
