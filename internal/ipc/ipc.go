package ipc

import (
	"fmt"
	app "lnn/internal"
	"lnn/internal/network"
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

func SendErr(conn net.Conn, code Code, err error) {
	Send(conn, Response{Code: code, Err: err.Error()})
}

func SendOk(conn net.Conn) {
	Send(conn, Response{Code: OK, Err: ""})
}

func Listen(s app.Services, srv *network.Server) error {
	path := os.Getenv("LNND_IPC_SOCKET")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: could not remove socket file %s: %v", path, err)
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0600); err != nil {
        ln.Close()
    	return fmt.Errorf("restrict IPC socket permissions: %w", err)
    }
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Error accepting IPC connection: %v", err)
			continue
		}
		go HandleConn(conn, s, srv)
	}
}
