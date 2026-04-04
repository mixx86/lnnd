package main

import (
	"lnn/internal/peer"
	"lnn/internal/server"
	"log"
)

func main() {
	id, err := peer.InitUUID()
	if err != nil {
		log.Fatalf("Error trying to resolve uuid: %v", err)
	}
	server.Listen(id)
}
