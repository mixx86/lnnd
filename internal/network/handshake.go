package network

import (
	"fmt"
	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
)

// OutBoundHandshake authenticates this node to a remote peer as the initiating side.
func OutboundHandshake(conn net.Conn, i *identity.IdentityService, sm *peer.SessionManager) (*peer.Session, error) {
	creds := i.Creds()
	nonce, err := protocol.GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("error generating the nonce")
	}

	err = sendRaw(conn, protocol.Frame{
		Body: protocol.Hello{
			ID:        creds.ID,
			PublicKey: creds.PubKey,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error sending the HELLO message")
	}
	err = sendRaw(conn, protocol.Frame{
		Body: protocol.Challenge{
			Nonce: nonce,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error sending the CHALLENGE message")
	}

	msg, err := ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the response")
	}
	hello, ok := msg.Body.(protocol.Hello)
	if !ok {
		return nil, fmt.Errorf("error converting the response")
	}

	msg, err = ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the CHALLENGE message")
	}
	challenge, ok := msg.Body.(protocol.Challenge)
	if !ok {
		return nil, fmt.Errorf("error converting the response")
	}

	msg, err = ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the CHALLENGE_RESP message")
	}
	challengeResp, ok := msg.Body.(protocol.ChallengeResp)
	if !ok {
		return nil, fmt.Errorf("invalid type of data: %v", err)
	}
	ok = protocol.VerifyKey(hello.PublicKey, nonce, challengeResp.Signature)
	if !ok {
		return nil, fmt.Errorf("error verifying the key")
	}

	// Verify the peer's signature before sending ours — never sign for an unauthenticated peer.
	signature := protocol.ResolveNonce(creds.PrivKey, challenge.Nonce)
	err = sendRaw(conn, protocol.Frame{
		Body: protocol.ChallengeResp{
			Signature: signature,
		},
	})
	if err != nil {
		return nil, err
	}

	session := peer.NewSession(hello.ID, hello.PublicKey, conn)

	sm.Register(
		session,
	)
	return session, nil
}

// InBoundHandshake authenticates an incoming connection as the accepting side.
// Returns a verified Peer registered in pm.
func InboundHandshake(conn net.Conn, i *identity.IdentityService, pm *peer.SessionManager) (*peer.Session, error) {
	msg, err := ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the HELLO message")
	}
	hello, ok := msg.Body.(protocol.Hello)
	if !ok {
		return nil, fmt.Errorf("error converting the response")
	}

	msg, err = ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the CHALLENGE message")
	}
	challenge, ok := msg.Body.(protocol.Challenge)
	if !ok {
		return nil, fmt.Errorf("error converting the response")
	}

	nonce, err := protocol.GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("error generating the nonce")
	}

	creds := i.Creds()
	err = sendRaw(conn, protocol.Frame{
		Body: protocol.Hello{
			ID:        creds.ID,
			PublicKey: creds.PubKey,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error sending the HELLO message")
	}

	err = sendRaw(conn, protocol.Frame{
		Body: protocol.Challenge{Nonce: nonce},
	})
	if err != nil {
		return nil, fmt.Errorf("error sending the CHALLENGE message")
	}

	signature := protocol.ResolveNonce(creds.PrivKey, challenge.Nonce)
	err = sendRaw(conn, protocol.Frame{
		Body: protocol.ChallengeResp{
			Signature: signature,
		},
	})
	if err != nil {
		return nil, err
	}

	msg, err = ReceiveTCP(conn)
	if err != nil {
		return nil, fmt.Errorf("error receiving the CHALLENGE_RESP message")
	}
	challengeResp, ok := msg.Body.(protocol.ChallengeResp)
	if !ok {
		return nil, fmt.Errorf("invalid type of data: %v", err)
	}

	ok = protocol.VerifyKey(hello.PublicKey, nonce, challengeResp.Signature)
	if !ok {
		return nil, fmt.Errorf("error verifying the key")
	}

	// TODO: Verify if there's already a session with the same id/key
	session := peer.NewSession(hello.ID, hello.PublicKey, conn)

	pm.Register(session)
	return session, nil
}
