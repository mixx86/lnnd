// Package config provides configuration loading utilities for the LNN daemon.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lnn"), nil
}

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

func parseBytes(s string) (uint64, bool) {
	mult := uint64(1)
	switch {
	case strings.HasSuffix(s, "KB"), strings.HasSuffix(s, "kb"):
		mult, s = 1024, s[:len(s)-2]
	case strings.HasSuffix(s, "MB"), strings.HasSuffix(s, "mb"):
		mult, s = 1024*1024, s[:len(s)-2]
	case strings.HasSuffix(s, "GB"), strings.HasSuffix(s, "gb"):
		mult, s = 1024*1024*1024, s[:len(s)-2]
	}

	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}

	return n * mult, true
}

const defaultMaxMem uint64 = 64 * 1024 * 1024

func MaxMemFromEnv() uint64 {
	s := os.Getenv("LNND_MAX_MEM")
	if s == "" {
		return defaultMaxMem
	}

	n, ok := parseBytes(s)
	if !ok || n == 0 {
		return defaultMaxMem
	}

	return n
}
