package config

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		Transport: "tcp",
		TCP: TCPConfig{
			Port:            os.Getenv("TCP_PORT"),
			KeepAlive:       true,
			KeepAlivePeriod: 30 * time.Second,
		},
		QUIC: QUICConfig{
			Port:            os.Getenv("QUIC_PORT"),
			KeepAlivePeriod: 30 * time.Second,
			MaxIdleTimeout:  60 * time.Second,
		},
	}
}

type TCPConfig struct {
	Port            string
	KeepAlive       bool
	KeepAlivePeriod time.Duration
}

type QUICConfig struct {
	Port            string
	KeepAlivePeriod time.Duration
	MaxIdleTimeout  time.Duration
}

type NetworkConfig struct {
	Transport string

	TCP  TCPConfig
	QUIC QUICConfig
}

func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lnn"), nil
}

// ConfigPath lives in the data dir so the daemon finds it regardless of the
// working directory it was started from.
func ConfigPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lnnd.conf"), nil
}

// EnsureConfig reports whether it created the file. O_EXCL keeps an
// existing config untouched even if two daemons start at the same time.
func EnsureConfig(path string, defaults []byte) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := f.Write(defaults); err != nil {
		_ = f.Close()
		return false, err
	}
	return true, f.Close()
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

// RequestsQueueMaxBytes defaults to a quarter of LNND_MAX_MEM: it leaves
// room for the files queue, and it has to trigger before the requests
// channel fills with long paths (~34 MB worst case), otherwise the limit
// would be redundant with ErrOverloaded.
func RequestsQueueMaxBytes() int64 {
	if n, ok := parseBytes(os.Getenv("LNND_REQ_QUEUE_MAX_BYTES")); ok {
		return int64(n)
	}
	return int64(MaxMemFromEnv() / 4)
}
