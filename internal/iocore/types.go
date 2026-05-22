package iocore

import (
	"lnn/internal/peer"
	"lnn/internal/protocol"
)

// PendingRequest is transient: it exists only while the WAL writer is persisting
// the request to disk. Once Written is closed, this struct must not be retained —
// the on-disk file is the sole source of truth for pending requests.
// From is needed by the WAL writer to record the peer UUID.
type PendingRequest struct {
	ID      uint32
	From    *peer.Peer
	Req     protocol.InboundRequest
	Written chan struct{}
}
