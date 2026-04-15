package ipc

import (
	"encoding/json"
	"log"
)

// Request represents an IPC request message.
type Request struct {
	Op   string `json:"op"`   // Operation type (e.g., "link")
	Type string `json:"type"` // Link type (e.g., "sym", "hard")
	Src  string `json:"src"`  // Source path in IP://path format
	Dst  string `json:"dst"`  // Destination path
	IP   string `json:"-"`    // Parsed IP address (not serialized)
}

// Response represents an IPC response message.
type Response struct {
	Status string `json:"status"` // Status code or message
	Msg    string `json:"msg"`    // Human-readable message
}

// ParseRequest deserializes a JSON request from bytes.
func ParseRequest(data []byte) (Request, error) {
	var req Request
	err := json.Unmarshal(data, &req)
	if err != nil {
		log.Fatalf("Error deserializing json data from the request: %v", err)
		return req, err
	}
	return req, err
}
