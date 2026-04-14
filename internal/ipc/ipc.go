package ipc

import (
	"io"
	"log"
	"net"
	"os"
)

func handleConnUNIX(conn net.Conn) {
	defer conn.Close()
	data, err := io.ReadAll(conn)
	if err != nil {
		log.Fatalf("Error receiving data from the peer: %v", err)
	}
	req, err := ParseRequest(data)
	if err != nil {
		log.Fatalf("Error deserializing json data: %v", err)
	}
	HandleRequest(&req)
}

func ListenIPC() {
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
		go handleConnUNIX(conn)
	}
}
