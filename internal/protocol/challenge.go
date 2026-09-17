// Package protocol implements the LNN wire protocol messages and encoding.
package protocol

const (
	UUIDSize      = 16
	PublicKeySize = 32
	NonceSize     = 32
)

// ChallengePayloadSize is the size in bytes of a challenge message payload.
const ChallengePayloadSize uint32 = 32

// ChallengeMessage contains a cryptographic nonce for peer authentication.
type Challenge struct {
	Nonce [ChallengePayloadSize]byte
}

// Encode serializes the challenge message into wire format.
func (c Challenge) Encode() []byte {
	return EncodeChallenge(c.Nonce)
}

func EncodeChallenge(nonce [NonceSize]byte) []byte {
	payload := make([]byte, ChallengePayloadSize)
	copy(payload, nonce[:])
	return payload
}

// ParseChallenge deserializes a challenge message from raw bytes.
func ParseChallenge(payload []byte) (Challenge, error) {
	var parsedPayload Challenge
	copy(parsedPayload.Nonce[:], payload)
	return parsedPayload, nil
}

func (Challenge) Type() MsgKind { return MsgChallenge }
func (Challenge) Size() uint32  { return ChallengePayloadSize }

// ChallengeRespPayloadSize is the size in bytes of a challenge response (Ed25519 signature).
const ChallengeRespPayloadSize uint32 = 64

// ChallengeRespMessage contains a signature over the received nonce.
type ChallengeResp struct {
	Signature [ChallengeRespPayloadSize]byte
}

// Encode serializes the challenge response message into wire format.
func (c ChallengeResp) Encode() []byte {
	return EncodeChallengeResp(c.Signature)
}

// EncodeChallengeResp constructs a complete challenge response message packet.
func EncodeChallengeResp(sig [ChallengeRespPayloadSize]byte) []byte {
	packet := make([]byte, ChallengeRespPayloadSize)

	copy(packet[:], sig[:])

	return packet
}

// ParseChallengeResp deserializes a challenge response message from raw bytes.
func ParseChallengeResp(payload []byte) (ChallengeResp, error) {
	var parsedPayload ChallengeResp
	copy(parsedPayload.Signature[:], payload)
	return parsedPayload, nil
}

func (ChallengeResp) Type() MsgKind { return MsgChallengeResp }
func (ChallengeResp) Size() uint32  { return ChallengeRespPayloadSize }
