package ipc

import (
	"io"
	app "lnn/internal"
	"log"
	"net"
	"os"
)

// One request per connection — each CLI invocation opens a fresh socket.
func HandleConnUNIX(conn net.Conn, s app.Services) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing IPC connection: %v", err)
		}
	}()
	data, err := io.ReadAll(conn)
	if err != nil {
		log.Printf("Error receiving data from the peer: %v", err)
		return
	}
	req, err := ParseRequest(data)
	if err != nil {
		log.Printf("Error deserializing json data: %v", err)
		return
	}
	HandleRequest(&req, conn, s)
}

// ListenIPC starts the Unix socket listener. Reads the path from LNND_IPC_SOCKET. Blocks indefinitely.
func ListenIPC(s app.Services) error {
	path := os.Getenv("LNND_IPC_SOCKET")
	// Remove any stale socket file left by a previous unclean shutdown; net.Listen would
	// fail with "address already in use" otherwise even though nothing is listening.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: could not remove socket file %s: %v", path, err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Error accepting IPC connection: %v", err)
			continue
		}
		go HandleConnUNIX(conn, s)
	}
}
