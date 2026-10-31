package main

import (
	"lnn/configs"
	"lnn/internal/config"
	"lnn/internal/daemon"
	"log"
)

func main() {
	path, err := config.ConfigPath()
	if err != nil {
		log.Fatalf("Couldn't locate config: %v", err)
	}
	created, err := config.EnsureConfig(path, configs.Default)
	if err != nil {
		log.Fatalf("Couldn't create default config: %v", err)
	}
	if created {
		log.Printf("Created default config at %s", path)
	}
	if err := config.LoadEnv(path); err != nil {
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
