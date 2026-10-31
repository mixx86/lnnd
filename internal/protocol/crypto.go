package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
)

func GenerateNonce() ([NonceSize]byte, error) {
	var nonce [NonceSize]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return [NonceSize]byte{}, err
	}
	return nonce, nil
}

func ResolveNonce(
	privKey ed25519.PrivateKey,
	nonce [NonceSize]byte,
) [ChallengeRespPayloadSize]byte {
	return [ChallengeRespPayloadSize]byte(ed25519.Sign(privKey, nonce[:]))
}

func VerifyKey(
	pubKey ed25519.PublicKey,
	nonce [NonceSize]byte,
	sig [ChallengeRespPayloadSize]byte,
) bool {
	return ed25519.Verify(pubKey, nonce[:], sig[:])
}
