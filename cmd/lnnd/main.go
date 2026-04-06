package main

import (
	"lnn/internal/config"
	"lnn/internal/peer"
	"lnn/internal/server"
	"log"
)

func main() {
	config.Load("configs/lnnd.conf")
	id, err := peer.InitUUID()
	if err != nil {
		log.Fatalf("Error trying to resolve uuid: %v", err)
	}
	go server.ListenIPC(id)
	server.ListenTCP(id)
}
