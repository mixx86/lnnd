package config

import (
	"bufio"
	"log"
	"os"
	"strings"
)

func Load(path string) {
	env_file, err := os.Open(path)
	if err != nil {
		log.Fatalf("Error opening env file: %v", err)
	}
	scanner := bufio.NewScanner(env_file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}
}
