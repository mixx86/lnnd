package network

import (
	"fmt"
	"lnn/internal/identity"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"net"
)

func OutBoundHandshake(conn net.Conn, i *identity.IdentityService, p *peer.PeerManager) error {
	// Receiving the HELLO message
	parsedHelloMsg, err := RecvTCP(conn)
	helloMsg, ok := parsedHelloMsg.(protocol.HelloMessage)
	if !ok {
		return fmt.Errorf("Error converting the response")
	}
	// Receiving the CHALLENGE message
	parsedChallengeMsg, err := RecvTCP(conn)
	challengeMsg, ok := parsedChallengeMsg.(protocol.ChallengeMessage)
	if !ok {
		return fmt.Errorf("Error converting the response")
	}

	nonce, err := protocol.GenerateNonce()

	creds := i.Creds()
	// Replying the HELLO messsage
	err = SendTCP(conn, protocol.HelloMessage{
		ID:        creds.ID,
		PublicKey: creds.PubKey,
	})
	if err != nil {
		return fmt.Errorf("Error sending the HELLO message")
	}
	// Replying the CHALLENGE message
	err = SendTCP(conn, protocol.ChallengeMessage{Nonce: nonce})
	if err != nil {
		return fmt.Errorf("Error generating the nonce")
	}

	// Resolve and send challenge
	signature := protocol.ResolveNonce(creds.PrivKey, challengeMsg.Nonce)
	err = SendTCP(conn, protocol.ChallengeRespMessage{
		Signature: signature,
	})
	if err != nil {
		return err
	}

	// Receiving the new public key made with our nonce
	parsedChallengeRespMsg, err := RecvTCP(conn)
	challengeRespMsg, ok := parsedChallengeRespMsg.(protocol.ChallengeRespMessage)
	if !ok {
		return fmt.Errorf("Invalid type of data: %v", err)
	}
	ok = protocol.VerifyKey(helloMsg.PublicKey, nonce, challengeRespMsg.Signature)
	if !ok {
		return fmt.Errorf("Error verifying the key")
	}

	p.Register(&peer.PeerConn{
		ID:     helloMsg.ID,
		PubKey: helloMsg.PublicKey,
		Conn:   conn,
	})
	return nil
}
