package identity

import (
	"crypto/ed25519"

	"github.com/google/uuid"
)

// Credentials holds the node's identity credentials.
type Credentials struct {
	ID      uuid.UUID          // Stable node identifier, persisted across restarts.
	PrivKey ed25519.PrivateKey // Used to sign challenge nonces during handshake.
	PubKey  ed25519.PublicKey  // Shared with peers during HELLO so they can verify our signatures.
}

// IdentityService manages node identity and credentials
type Service struct {
	creds Credentials // Unexported to prevent external mutation after initialization.
}
