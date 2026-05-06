package main

import (
	"lnn/internal/config"
	"lnn/internal/daemon"
	"log"
)

func main() {
	if err := config.LoadEnv("configs/lnnd.conf"); err != nil {
		log.Fatalf("Couldn't load config: %v", err)
	}
	d, err := daemon.New()
	if err != nil {
		log.Fatalf("Couldn't create the daemon: %v", err)
	}
	if err := d.Start(); err != nil {
		log.Fatalf("Daemon failed: %v", err)
	}
}
