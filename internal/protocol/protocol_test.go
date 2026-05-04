package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/google/uuid"
)

// --- Header ---

func TestReadHeader_valid(t *testing.T) {
	msg := EncodeHello(uuid.New(), make(ed25519.PublicKey, PublicKeySize))
	h, err := ReadHeader(bytes.NewReader(msg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Type != MsgHello {
		t.Errorf("expected type %d, got %d", MsgHello, h.Type)
	}
	if h.PayloadLength != HelloPayloadSize {
		t.Errorf("expected payload length %d, got %d", HelloPayloadSize, h.PayloadLength)
	}
}

func TestReadHeader_badMagic(t *testing.T) {
	msg := EncodeHello(uuid.New(), make(ed25519.PublicKey, PublicKeySize))
	msg[0] = 0xFF
	_, err := ReadHeader(bytes.NewReader(msg))
	if err == nil {
		t.Fatal("expected error for bad magic, got nil")
	}
}

func TestReadHeader_badVersion(t *testing.T) {
	msg := EncodeHello(uuid.New(), make(ed25519.PublicKey, PublicKeySize))
	msg[OffsetVersion] = 0xFF
	_, err := ReadHeader(bytes.NewReader(msg))
	if err == nil {
		t.Fatal("expected error for bad version, got nil")
	}
}

// --- Hello ---

func TestHello_roundtrip(t *testing.T) {
	id := uuid.New()
	pub := make(ed25519.PublicKey, PublicKeySize)
	rand.Read(pub)

	parsed, err := ParseHello(EncodeHello(id, pub)[OffsetBody:])
	if err != nil {
		t.Fatalf("ParseHello: %v", err)
	}
	if parsed.ID != id {
		t.Errorf("ID mismatch: got %v, want %v", parsed.ID, id)
	}
	if !bytes.Equal(parsed.PublicKey, pub) {
		t.Error("PublicKey mismatch")
	}
}

func TestHello_invalidPayloadSize(t *testing.T) {
	_, err := ParseHello(make([]byte, 10))
	if err == nil {
		t.Fatal("expected error for wrong payload size")
	}
}

// --- Challenge ---

func TestChallenge_roundtrip(t *testing.T) {
	var nonce [NonceSize]byte
	rand.Read(nonce[:])

	parsed, err := ParseChallenge(EncodeChallenge(nonce)[OffsetBody:])
	if err != nil {
		t.Fatalf("ParseChallenge: %v", err)
	}
	if parsed.Nonce != nonce {
		t.Error("Nonce mismatch")
	}
}

func TestChallenge_invalidPayloadSize(t *testing.T) {
	_, err := ParseChallenge(make([]byte, 10))
	if err == nil {
		t.Fatal("expected error for wrong payload size")
	}
}

// --- ChallengeResp ---

func TestChallengeResp_roundtrip(t *testing.T) {
	var sig [ChallengeRespPayloadSize]byte
	rand.Read(sig[:])

	parsed, err := ParseChallengeResp(EncodeChallengeResp(sig)[OffsetBody:])
	if err != nil {
		t.Fatalf("ParseChallengeResp: %v", err)
	}
	if parsed.Signature != sig {
		t.Error("Signature mismatch")
	}
}

// --- Error ---

func TestError_roundtrip(t *testing.T) {
	msg := ErrorMessage{Code: ErrInvalidState, Message: MsgErrInvalidState}
	packet := EncodeErrorMessage(msg.Code, msg.Message)

	parsed, err := ParseError(packet[OffsetBody:])
	if err != nil {
		t.Fatalf("ParseError: %v", err)
	}
	if parsed.Code != msg.Code {
		t.Errorf("Code mismatch: got %d, want %d", parsed.Code, msg.Code)
	}
	if parsed.Message != msg.Message {
		t.Errorf("Message mismatch: got %q, want %q", parsed.Message, msg.Message)
	}
}

func TestError_messageTruncated(t *testing.T) {
	long := "this message is definitely longer than thirty five bytes total"
	packet := EncodeErrorMessage(-1, long)
	parsed, err := ParseError(packet[OffsetBody:])
	if err != nil {
		t.Fatalf("ParseError: %v", err)
	}
	if len(parsed.Message) > ErrorMessageSize {
		t.Errorf("message not truncated: len=%d", len(parsed.Message))
	}
}

func TestError_headerType(t *testing.T) {
	packet := EncodeErrorMessage(ErrInvalidMessage, MsgErrInvalidMessage)
	if packet[OffsetType] != MsgError {
		t.Errorf("header type: got %d, want %d", packet[OffsetType], MsgError)
	}
}

func TestError_invalidPayloadSize(t *testing.T) {
	_, err := ParseError(make([]byte, 10))
	if err == nil {
		t.Fatal("expected error for wrong payload size")
	}
}

// --- Crypto ---

func TestGenerateNonce_unique(t *testing.T) {
	a, _ := GenerateNonce()
	b, _ := GenerateNonce()
	if a == b {
		t.Error("two nonces should not be equal")
	}
}

func TestVerifyKey_valid(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	nonce, _ := GenerateNonce()
	sig := ResolveNonce(priv, nonce)
	if !VerifyKey(pub, nonce, sig) {
		t.Error("valid signature should verify")
	}
}

func TestVerifyKey_wrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)
	nonce, _ := GenerateNonce()
	sig := ResolveNonce(priv, nonce)
	if VerifyKey(pub2, nonce, sig) {
		t.Error("signature should not verify with wrong key")
	}
}

func TestVerifyKey_wrongNonce(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	nonce, _ := GenerateNonce()
	other, _ := GenerateNonce()
	sig := ResolveNonce(priv, nonce)
	if VerifyKey(pub, other, sig) {
		t.Error("signature should not verify with different nonce")
	}
}
