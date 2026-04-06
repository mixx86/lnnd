package server

import (
	"io"
	"lnn/internal/ipc"
	"log"
	"net"
	"os"

	"github.com/google/uuid"
)

func handleConnUNIX(conn net.Conn, id uuid.UUID) {
	defer conn.Close()
	data, err := io.ReadAll(conn)
	if err != nil {
		log.Fatalf("Error receiving data from the peer: %v", err)
	}
	req, err := ipc.ParseRequest(data)
	if err != nil {
		log.Fatalf("Error deserializing json data: %v", err)
	}
	ipc.HandleRequest(&req, id)
}

func ListenIPC(id uuid.UUID) {
	path := os.Getenv("LNND_IPC_SOCKET")
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		log.Fatalf("Error listening to the connection: %v", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalf("Error accepting the connection: %v", err)
		}
		go handleConnUNIX(conn, id)
	}
}
