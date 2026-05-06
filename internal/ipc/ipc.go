package ipc

import (
	"io"
	"log"
	"net"
	"os"
)

// HandleConnUNIX reads a single request from conn, dispatches it, and closes the connection.
// Each CLI invocation opens a new connection, so the lifetime is one request per connection.
func HandleConnUNIX(conn net.Conn) {
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
	HandleRequest(&req)
}

// ListenIPC starts the Unix socket listener for IPC communication.
// Reads the socket path from LNND_IPC_SOCKET environment variable.
// This function blocks indefinitely.
func ListenIPC() error {
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
		go HandleConnUNIX(conn)
	}
}
