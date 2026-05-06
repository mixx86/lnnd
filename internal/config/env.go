// Package config provides configuration loading utilities for the LNN daemon.
package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnv reads KEY=VALUE pairs from path and sets them as environment variables.
// Lines without '=' are silently skipped. Returns an error if the file cannot
// be opened or if any variable cannot be set.
func LoadEnv(path string) (retErr error) {
	envFile, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := envFile.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()
	scanner := bufio.NewScanner(envFile)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			if err := os.Setenv(parts[0], parts[1]); err != nil {
				return err
			}
		}
	}
	return nil
}
