package identity

import (
	"crypto/ed25519"

	"github.com/google/uuid"
)

type Credentials struct {
	ID      uuid.UUID // persisted across restarts
	PrivKey ed25519.PrivateKey
	PubKey  ed25519.PublicKey
}

type Service struct {
	creds Credentials // unexported: immutable after initialization
}
