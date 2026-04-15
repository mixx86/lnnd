// Package config provides configuration loading utilities for the LNN daemon.
package config

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// LoadEnv reads environment variables from a file and sets them in the process.
// The file format is KEY=VALUE, one per line.
// Fatal error if the file cannot be opened.
func LoadEnv(path string) {
	envFile, err := os.Open(path)
	if err != nil {
		log.Fatalf("Error opening env file: %v", err)
	}
	scanner := bufio.NewScanner(envFile)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}
}
