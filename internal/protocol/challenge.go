package protocol

const (
	UUIDSize      = 16
	PublicKeySize = 32
	NonceSize     = 32
)

const ChallengePayloadSize uint32 = 32

type Challenge struct {
	Nonce [ChallengePayloadSize]byte
}

func (c Challenge) Encode() []byte {
	return EncodeChallenge(c.Nonce)
}

func EncodeChallenge(nonce [NonceSize]byte) []byte {
	payload := make([]byte, ChallengePayloadSize)
	copy(payload, nonce[:])
	return payload
}

func ParseChallenge(payload []byte) (Challenge, error) {
	var parsedPayload Challenge
	copy(parsedPayload.Nonce[:], payload)
	return parsedPayload, nil
}

func (Challenge) Type() MsgType { return MsgChallenge }
func (Challenge) Size() uint32  { return ChallengePayloadSize }

const ChallengeRespPayloadSize uint32 = 64

type ChallengeResp struct {
	Signature [ChallengeRespPayloadSize]byte
}

func (c ChallengeResp) Encode() []byte {
	return EncodeChallengeResp(c.Signature)
}

func EncodeChallengeResp(sig [ChallengeRespPayloadSize]byte) []byte {
	packet := make([]byte, ChallengeRespPayloadSize)

	copy(packet[:], sig[:])

	return packet
}

func ParseChallengeResp(payload []byte) (ChallengeResp, error) {
	var parsedPayload ChallengeResp
	copy(parsedPayload.Signature[:], payload)
	return parsedPayload, nil
}

func (ChallengeResp) Type() MsgType { return MsgChallengeResp }
func (ChallengeResp) Size() uint32  { return ChallengeRespPayloadSize }
