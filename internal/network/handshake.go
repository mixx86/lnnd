package network

import (
	"fmt"
	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
)

// OutBoundHandshake performs the client side of the authentication handshake.
// Sends HELLO and CHALLENGE first, then receives and verifies peer credentials.
func OutBoundHandshake(conn net.Conn, i *identity.IdentityService, p *peer.PeerManager) error {
	creds := i.Creds()
	nonce, err := protocol.GenerateNonce()
	if err != nil {
		return fmt.Errorf("Error generating the nonce")
	}

	// Send our HELLO message
	err = SendTCP(conn, protocol.HelloMessage{
		ID:        creds.ID,
		PublicKey: creds.PubKey,
	})
	// Send our CHALLENGE message
	err = SendTCP(conn, protocol.ChallengeMessage{
		Nonce: nonce,
	})

	// Receive peer's HELLO message
	HelloMsg, err := ReceiveTCP(conn)
	helloMsg, ok := HelloMsg.(protocol.HelloMessage)
	if !ok {
		return fmt.Errorf("Error converting the response")
	}
	// Receive peer's CHALLENGE message
	ChallengeMsg, err := ReceiveTCP(conn)
	challengeMsg, ok := ChallengeMsg.(protocol.ChallengeMessage)
	if !ok {
		return fmt.Errorf("Error converting the response")
	}

	// Receive peer's challenge response
	ChallengeRespMsg, err := ReceiveTCP(conn)
	challengeRespMsg, ok := ChallengeRespMsg.(protocol.ChallengeRespMessage)
	if !ok {
		return fmt.Errorf("Invalid type of data: %v", err)
	}
	ok = protocol.VerifyKey(helloMsg.PublicKey, nonce, challengeRespMsg.Signature)
	if !ok {
		return fmt.Errorf("Error verifying the key")
	}

	// Sign peer's challenge nonce and respond
	signature := protocol.ResolveNonce(creds.PrivKey, challengeMsg.Nonce)
	err = SendTCP(conn, protocol.ChallengeRespMessage{
		Signature: signature,
	})
	if err != nil {
		return err
	}

	// Peer authenticated successfully, register connection
	p.Register(&peer.Peer{
		ID:     helloMsg.ID,
		PubKey: helloMsg.PublicKey,
		Conn:   conn,
		State:  peer.StateVerified,
	})
	return nil
}

// InBoundHandshake performs the server side of the authentication handshake.
// Receives HELLO and CHALLENGE, sends own credentials, verifies peer identity.
func InBoundHandshake(conn net.Conn, i *identity.IdentityService, pm *peer.PeerManager) (peer.Peer, error) {
	// Receive the HELLO message
	HelloMsg, err := ReceiveTCP(conn)
	helloMsg, ok := HelloMsg.(protocol.HelloMessage)
	if !ok {
		return peer.Peer{}, fmt.Errorf("Error converting the response")
	}
	// Receive the CHALLENGE message
	ChallengeMsg, err := ReceiveTCP(conn)
	challengeMsg, ok := ChallengeMsg.(protocol.ChallengeMessage)
	if !ok {
		return peer.Peer{}, fmt.Errorf("Error converting the response")
	}

	// Preparing the challenge
	nonce, err := protocol.GenerateNonce()
	if err != nil {
		return peer.Peer{}, fmt.Errorf("Error generating the nonce")
	}

	// Loading our creds
	creds := i.Creds()
	// Send our HELLO message
	err = SendTCP(conn, protocol.HelloMessage{
		ID:        creds.ID,
		PublicKey: creds.PubKey,
	})
	if err != nil {
		return peer.Peer{}, fmt.Errorf("Error sending the HELLO message")
	}
	// Send our CHALLENGE message
	err = SendTCP(conn, protocol.ChallengeMessage{Nonce: nonce})
	if err != nil {
		return peer.Peer{}, fmt.Errorf("Error generating the nonce")
	}

	// Sign peer's challenge nonce
	signature := protocol.ResolveNonce(creds.PrivKey, challengeMsg.Nonce)
	err = SendTCP(conn, protocol.ChallengeRespMessage{
		Signature: signature,
	})
	if err != nil {
		return peer.Peer{}, err
	}

	// Receive peer's challenge response
	ChallengeRespMsg, err := ReceiveTCP(conn)
	challengeRespMsg, ok := ChallengeRespMsg.(protocol.ChallengeRespMessage)
	if !ok {
		return peer.Peer{}, fmt.Errorf("Invalid type of data: %v", err)
	}
	ok = protocol.VerifyKey(helloMsg.PublicKey, nonce, challengeRespMsg.Signature)
	if !ok {
		return peer.Peer{}, fmt.Errorf("Error verifying the key")
	}

	p := peer.Peer{
		ID:     helloMsg.ID,
		PubKey: helloMsg.PublicKey,
		Conn:   conn,
		State:  peer.StateVerified,
	}
	// Peer authenticated successfully, register connection
	pm.Register(&p)
	return p, nil
}
