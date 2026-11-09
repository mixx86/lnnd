package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"lnn/internal/config"
	"lnn/internal/logger"
)

func NewService() (*Service, error) {
	creds, err := LoadLocalCredentials()

	if err != nil {
		return &Service{}, err
	}
	return &Service{
		creds: creds,
	}, nil
}

func (i *Service) Credentials() Credentials {
	return i.creds
}

func LoadLocalCredentials() (Credentials, error) {
	id, err := InitUUID()
	if err != nil {
		return Credentials{}, err
	}
	priv, pub, err := InitKeys()
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{ID: id, PrivKey: priv, PubKey: pub}, nil
}

func InitUUID() (uuid.UUID, error) {
	var id uuid.UUID
	dir, err := config.DataDir()
	if err != nil {
		return id, fmt.Errorf("failed to get data dir: %w", err)
	}
	path := filepath.Join(dir, "uuid")
	// 0700: the directory holds private key material.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return id, fmt.Errorf("failed to create dir: %w", err)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		id, err = uuid.Parse(strings.TrimSpace(string(data)))
		if err != nil {
			return id, fmt.Errorf("failed to parse uuid: %w", err)
		}
		logger.Daemon.Printf("UUID loaded: %s", id)
		return id, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		id = uuid.New()
		if err := os.WriteFile(path, []byte(id.String()), 0600); err != nil {
			return id, fmt.Errorf("failed to write uuid: %w", err)
		}
		log.Printf("New UUID generated: %s", id)
		return id, nil
	}
	return id, fmt.Errorf("unexpected error: %w", err)
}

func InitKeys() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	dir, err := config.DataDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get data dir: %w", err)
	}
	privPath := filepath.Join(dir, "id_ed25519")
	pubPath := filepath.Join(dir, "id_ed25519.pub")
	privData, err := os.ReadFile(privPath)
	if err == nil {
		// ed25519 panics on a key of any other size, so a torn first write
		// or a key in another format would crash the daemon at its first
		// handshake instead of failing here.
		if len(privData) != ed25519.PrivateKeySize {
			return nil, nil, fmt.Errorf(
				"invalid private key %s: %d bytes, want %d",
				privPath,
				len(privData),
				ed25519.PrivateKeySize,
			)
		}
		priv := ed25519.PrivateKey(privData)
		pub := priv.Public().(ed25519.PublicKey)
		return priv, pub, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		// 0600: private key must never be readable by other users.
		if err := os.WriteFile(privPath, priv, 0600); err != nil {
			return nil, nil, fmt.Errorf("failed to save private key: %w", err)
		}
		// Base64 so the public key is easy to share and inspect.
		pubBase64 := base64.StdEncoding.EncodeToString(pub)
		// 0644: public key can be world-readable by design.
		err = os.WriteFile(pubPath, []byte(pubBase64+"\n"), 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to save public key: %w", err)
		}
		return priv, pub, nil
	}
	return nil, nil, err
}
