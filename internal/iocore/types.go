package iocore

import (
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"sync/atomic"
)

// PendingRequest is transient: it exists only while the WAL writer is persisting
// the request to disk. Once Written is closed, this struct must not be retained —
// the on-disk file is the sole source of truth for pending requests.
// From is needed by the WAL writer to record the peer UUID.
type PendingRequest struct {
	ID      uint32
	From    *peer.RemoteNode
	Req     protocol.InboundRequest
	Written chan struct{}
}

// Manager coordinates writers of the daemon to efficiently update changes on the disk
type Manager struct {
	reqCh         chan PendingRequest
	reqCounter    atomic.Uint32
	ioPressure    atomic.Int32
	pressureDone  chan struct{}
}
