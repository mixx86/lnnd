package ipc

import (
	"io"
	app "lnn/internal"
	"log"
	"net"
	"os"
)

func HandleConn(conn net.Conn, s app.Services) {
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

func ListenIPC(s app.Services) error {
	path := os.Getenv("LNND_IPC_SOCKET")
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
		go HandleConn(conn, s)
	}
}
