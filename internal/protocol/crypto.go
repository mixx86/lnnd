package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
)

// GenerateNonce creates a cryptographically random 32-byte nonce.
func GenerateNonce() ([NonceSize]byte, error) {
	var nonce [NonceSize]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return [NonceSize]byte{}, err
	}
	return nonce, nil
}

// ResolveNonce signs a nonce with the given private key and returns an Ed25519 signature.
func ResolveNonce(privKey ed25519.PrivateKey, nonce [NonceSize]byte) [ChallengeRespPayloadSize]byte {
	return [ChallengeRespPayloadSize]byte(ed25519.Sign(privKey, nonce[:]))
}

// VerifyKey verifies that sig was produced by signing nonce with the private key corresponding to pubKey.
func VerifyKey(pubKey ed25519.PublicKey, nonce [NonceSize]byte, sig [ChallengeRespPayloadSize]byte) bool {
	return ed25519.Verify(pubKey, nonce[:], sig[:])
}
