package ipc

import (
	"encoding/json"
	"log"
)

type Request struct {
	Op   string `json:"op"`
	Type string `json:"type"`
	Src  string `json:"src"`
	Dst  string `json:"dst"`
	IP   string `json:"-"` // internal use
}

type Response struct {
	Status string `json:"status"`
	Msg    string `json:"msg"`
}

func ParseRequest(data []byte) (Request, error) {
	var req Request
	err := json.Unmarshal(data, &req)
	if err != nil {
		log.Fatalf("Errror deserializing json data from the request: %v", err)
		return req, err
	}
	return req, err
}
