package ipc

import (
	"encoding/json"
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
	Code Code `json:"code"`
	Err string `json:"err,omitempty"`
}

func (r Response) Encode() ([]byte, error) {
	return json.Marshal(r)
}

type Request struct {
	Op   string `json:"op"`
	Type string `json:"type"`
	Src  string `json:"src"` // <ip>://<path> until ValidateRequest splits it
	Dst  string `json:"dst"`
	IP   string `json:"-"` // From ValidateRequest; never taken from the client
	UUID string `json:"uuid"`
}

func ParseMessage(data []byte) (Request, error) {
	var req Request
	err := json.Unmarshal(data, &req)
	return req, err
}
