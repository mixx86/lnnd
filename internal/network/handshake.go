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
		return fmt.Errorf("error generating the nonce")
	}

	err = sendRaw(conn, protocol.HelloMessage{
		ID:        creds.ID,
		PublicKey: creds.PubKey,
	})
	if err != nil {
		return fmt.Errorf("error sending the HELLO message")
	}
	err = sendRaw(conn, protocol.ChallengeMessage{
		Nonce: nonce,
	})
	if err != nil {
		return fmt.Errorf("error sending the CHALLENGE message")
	}

	HelloMsg, err := ReceiveTCP(conn)
	if err != nil {
		return fmt.Errorf("error receiving the response")
	}
	helloMsg, ok := HelloMsg.(protocol.HelloMessage)
	if !ok {
		return fmt.Errorf("error converting the response")
	}
	ChallengeMsg, err := ReceiveTCP(conn)
	if err != nil {
		return fmt.Errorf("error receiving the CHALLENGE message")
	}
	challengeMsg, ok := ChallengeMsg.(protocol.ChallengeMessage)
	if !ok {
		return fmt.Errorf("error converting the response")
	}

	ChallengeRespMsg, err := ReceiveTCP(conn)
	challengeRespMsg, ok := ChallengeRespMsg.(protocol.ChallengeRespMessage)
	if !ok {
		return fmt.Errorf("invalid type of data: %v", err)
	}
	ok = protocol.VerifyKey(helloMsg.PublicKey, nonce, challengeRespMsg.Signature)
	if !ok {
		return fmt.Errorf("error verifying the key")
	}

	signature := protocol.ResolveNonce(creds.PrivKey, challengeMsg.Nonce)
	err = sendRaw(conn, protocol.ChallengeRespMessage{
		Signature: signature,
	})
	if err != nil {
		return err
	}

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
func InBoundHandshake(conn net.Conn, i *identity.IdentityService, pm *peer.PeerManager) (*peer.Peer, error) {
	// Receive the HELLO message
	HelloMsg, err := ReceiveTCP(conn)
	helloMsg, ok := HelloMsg.(protocol.HelloMessage)
	if !ok {
		return nil,fmt.Errorf("Error converting the response")
	}
	// Receive the CHALLENGE message
	ChallengeMsg, err := ReceiveTCP(conn)
	challengeMsg, ok := ChallengeMsg.(protocol.ChallengeMessage)
	if !ok {
		return nil,fmt.Errorf("Error converting the response")
	}

	// Preparing the challenge
	nonce, err := protocol.GenerateNonce()
	if err != nil {
		return nil,fmt.Errorf("Error generating the nonce")
	}

	// Loading our creds
	creds := i.Creds()
	// Send our HELLO message
	err = sendRaw(conn, protocol.HelloMessage{
		ID:        creds.ID,
		PublicKey: creds.PubKey,
	})
	if err != nil {
		return nil,fmt.Errorf("Error sending the HELLO message")
	}
	// Send our CHALLENGE message
	err = sendRaw(conn, protocol.ChallengeMessage{Nonce: nonce})
	if err != nil {
		return nil,fmt.Errorf("Error generating the nonce")
	}

	// Sign peer's challenge nonce
	signature := protocol.ResolveNonce(creds.PrivKey, challengeMsg.Nonce)
	err = sendRaw(conn, protocol.ChallengeRespMessage{
		Signature: signature,
	})
	if err != nil {
		return nil,err
	}

	// Receive peer's challenge response
	ChallengeRespMsg, err := ReceiveTCP(conn)
	challengeRespMsg, ok := ChallengeRespMsg.(protocol.ChallengeRespMessage)
	if !ok {
		return nil,fmt.Errorf("Invalid type of data: %v", err)
	}
	ok = protocol.VerifyKey(helloMsg.PublicKey, nonce, challengeRespMsg.Signature)
	if !ok {
		return nil,fmt.Errorf("Error verifying the key")
	}

	p := peer.Peer{
		ID:     helloMsg.ID,
		PubKey: helloMsg.PublicKey,
		Conn:   conn,
		State:  peer.StateVerified,
	}
	// Peer authenticated successfully, register connection
	pm.Register(&p)
	return &p, nil
}
