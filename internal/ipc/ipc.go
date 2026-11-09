package ipc

import (
	app "lnn/internal"
	"log"
	"net"
	"os"
)

func Send(conn net.Conn, res Response) (int, error) {
	bytes, err := res.Encode()
	if err != nil {
		return 0, err
	}
	return conn.Write(bytes)
}

func Listen(s app.Services) error {
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
