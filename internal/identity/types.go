// Package identity types
package identity

import (
	"crypto/ed25519"

	"github.com/google/uuid"
)

// Credentials holds the node's identity credentials.
type Credentials struct {
	ID      uuid.UUID
	PrivKey ed25519.PrivateKey
	PubKey  ed25519.PublicKey
}

// IdentityService manages node identity and credentials.
type IdentityService struct {
	creds Credentials
}
