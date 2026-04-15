package main

import (
	"lnn/internal/config"
	"lnn/internal/daemon"
	"log"
)

func main() {
	config.LoadEnv("configs/lnnd.conf")
	d, err := daemon.New()
	if err != nil {
		log.Fatalf("Couldn't create the daemon: %v", err)
	}
	d.Start()
}
