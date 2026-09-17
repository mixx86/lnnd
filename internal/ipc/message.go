package ipc

import (
	"encoding/json"
)

// Request represents an IPC request message.
type Request struct {
	Op   string `json:"op"`   // Operation type (e.g., "link")
	Type string `json:"type"` // Link type (e.g., "sym", "hard")
	Src  string `json:"src"`  // Source path in IP://path format
	Dst  string `json:"dst"`  // Destination path
	IP   string `json:"-"`    // Extracted from Src by Validate; omitted from JSON since it is derived, not provided separately.
}

// Response represents an IPC response message.
type Response struct {
	Ok  bool   `json:"ok"`
	Err string `json:"err,omitempty"`
}

func (r Response) Encode() ([]byte, error) {
	return json.Marshal(r)
}

// ParseRequest deserializes a JSON request from bytes.
func ParseRequest(data []byte) (Request, error) {
	var req Request
	err := json.Unmarshal(data, &req)
	return req, err
}
