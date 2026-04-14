package identity

import (
	"crypto/ed25519"

	"github.com/google/uuid"
)

type LocalCreds struct {
	ID      uuid.UUID
	PrivKey ed25519.PrivateKey
	PubKey  ed25519.PublicKey
}

type IdentityService struct {
	creds LocalCreds
}
