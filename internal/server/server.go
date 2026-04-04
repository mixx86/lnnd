package server

import (
	"lnn/internal/protocol"
	"log"
	"net"

	"github.com/google/uuid"
)

func handleConn(conn net.Conn, id uuid.UUID) {
	packet := protocol.BuildHello(id)
	bytes, err := conn.Write(packet)
	if err != nil {
		log.Fatalf("Error sending data: %v", err)
	}
	log.Printf("Sent %v bytes", bytes)
	conn.Close()
}

func Listen(id uuid.UUID) {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Error opening the conexion: %v", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalf("Error accepting the conexion: %v", err)
		}
		go handleConn(conn, id)
	}
}
