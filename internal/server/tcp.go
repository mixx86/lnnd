package server

import (
	"lnn/internal/client"
	"lnn/internal/protocol"
	"log"
	"net"
	"os"

	"github.com/google/uuid"
)

func handleConnTCP(conn net.Conn, id uuid.UUID) {
	defer conn.Close()
	err := client.SendTCP(conn, protocol.HelloMessage{
		ID: id,
	})
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	remoteID, err := client.RecvTCP(conn)
	if err != nil {
		log.Fatalf("Couldn't receive the uuid: %v", err)
	}
	log.Printf("uuid received from icp server: %v", remoteID)
}

func ListenTCP(id uuid.UUID) {
	port := os.Getenv("TCP_PORT")
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Error opening the connection: %v", err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalf("Error accepting the connection: %v", err)
		}
		go handleConnTCP(conn, id)
	}
}
