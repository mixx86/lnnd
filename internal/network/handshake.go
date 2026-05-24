package network

import (
	"fmt"
	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
)

// OutBoundHandshake authenticates this node to a remote peer as the initiating side.
func OutBoundHandshake(conn net.Conn, i *identity.IdentityService, sm *peer.SessionManager) error {
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

	// Verify the peer's signature before sending ours — never sign for an unauthenticated peer.
	signature := protocol.ResolveNonce(creds.PrivKey, challengeMsg.Nonce)
	err = sendRaw(conn, protocol.ChallengeRespMessage{
		Signature: signature,
	})
	if err != nil {
		return err
	}

	sm.Register(
		peer.NewSession(helloMsg.ID, helloMsg.PublicKey, conn),
	)
	return nil
}

// InBoundHandshake authenticates an incoming connection as the accepting side.
// Returns a verified Peer registered in pm.
func InBoundHandshake(conn net.Conn, i *identity.IdentityService, pm *peer.SessionManager) (*peer.Session, error) {
	HelloMsg, err := ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the HELLO message")
	}
	helloMsg, ok := HelloMsg.(protocol.HelloMessage)
	if !ok {
		return nil, fmt.Errorf("error converting the response")
	}
	ChallengeMsg, err := ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the CHALLENGE message")
	}
	challengeMsg, ok := ChallengeMsg.(protocol.ChallengeMessage)
	if !ok {
		return nil, fmt.Errorf("error converting the response")
	}

	nonce, err := protocol.GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("error generating the nonce")
	}

	creds := i.Creds()
	err = sendRaw(conn, protocol.HelloMessage{
		ID:        creds.ID,
		PublicKey: creds.PubKey,
	})
	if err != nil {
		return nil, fmt.Errorf("error sending the HELLO message")
	}
	err = sendRaw(conn, protocol.ChallengeMessage{Nonce: nonce})
	if err != nil {
		return nil, fmt.Errorf("error generating the nonce")
	}

	signature := protocol.ResolveNonce(creds.PrivKey, challengeMsg.Nonce)
	err = sendRaw(conn, protocol.ChallengeRespMessage{
		Signature: signature,
	})
	if err != nil {
		return nil, err
	}

	ChallengeRespMsg, err := ReceiveTCP(conn)
	challengeRespMsg, ok := ChallengeRespMsg.(protocol.ChallengeRespMessage)
	if !ok {
		return nil, fmt.Errorf("invalid type of data: %v", err)
	}
	ok = protocol.VerifyKey(helloMsg.PublicKey, nonce, challengeRespMsg.Signature)
	if !ok {
		return nil, fmt.Errorf("error verifying the key")
	}

	session := peer.NewSession(helloMsg.ID, helloMsg.PublicKey, conn)
	
	pm.Register(session)
	return session, nil
}
