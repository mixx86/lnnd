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
)

func NewService() (*IdentityService, error) {
	creds, err := LoadLocalCreds()

	if err != nil {
		return &IdentityService{}, err
	}
	return &IdentityService{
		creds: creds,
	}, nil
}

func (i *IdentityService) Creds() LocalCreds {
	return i.creds
}

func LoadLocalCreds() (LocalCreds, error) {
	id, err := InitUUID()
	if err != nil {
		return LocalCreds{}, err
	}
	priv, pub, err := InitKeys()
	if err != nil {
		return LocalCreds{}, err
	}
	return LocalCreds{ID: id, PrivKey: priv, PubKey: pub}, nil
}

func InitUUID() (uuid.UUID, error) {
	var id uuid.UUID
	home, err := os.UserHomeDir()
	if err != nil {
		return id, fmt.Errorf("failed to get home dir: %w", err)
	}
	dir := filepath.Join(home, ".lnn")
	path := filepath.Join(dir, "uuid")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return id, fmt.Errorf("failed to create dir: %w", err)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		id, err = uuid.Parse(strings.TrimSpace(string(data)))
		if err != nil {
			return id, fmt.Errorf("failed to parse uuid: %w", err)
		}
		log.Printf("UUID loaded: %s", id)
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
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".lnn")
	privPath := filepath.Join(dir, "id_ed25519")
	pubPath := filepath.Join(dir, "id_ed25519.pub")
	privData, err := os.ReadFile(privPath)
	if err == nil {
		priv := ed25519.PrivateKey(privData)
		pub := priv.Public().(ed25519.PublicKey)
		return priv, pub, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		if err := os.WriteFile(privPath, priv, 0600); err != nil {
			return nil, nil, fmt.Errorf("failed to save private key: %w", err)
		}
		pubBase64 := base64.StdEncoding.EncodeToString(pub)
		if err := os.WriteFile(pubPath, []byte(pubBase64+"\n"), 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to save public key: %w", err)
		}
		return priv, pub, nil
	}
	return nil, nil, err
}
