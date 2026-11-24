package queues

import (
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"sync/atomic"

	"time"
)

type QueueID uint16

const (
	RequestsQueueID QueueID = iota
	FilesQueueID
)

// PendingRequest is transient: it exists only while the WAL writer is
// persisting the request to disk. Once Written is closed, this struct must
// not be retained — the on-disk file is the sole source of truth for pending
// requests.
type PendingRequest struct {
	Peer       *peer.RemoteNode
	Req        protocol.InboundRequest
	Written    chan WriteResult
	EnqueuedAt uint64
	Tag        uint32
	// Sent marks a request of ours, logged before it goes to the peer.
	Sent bool
}

// WriteResult carries the lcl_req_id the writer assigned: a request of ours
// needs it before it goes to the peer.
type WriteResult struct {
	LCLReqID uint32
	Err      error
}

type RequestsQueue struct {
	queue  chan PendingRequest
	config QueueConfig
}

type FilesQueue struct {
}

type QueueConfig struct {
	MaxQueuedBytes int64
	QueuedBytes    atomic.Int64
}

type Queue[T any, U any] interface {
	Dequeue() U
	TryDequeue() (U, bool)
	TryEnqueue(from *peer.RemoteNode, item T) (U, bool)
	TryEnqueueWithTimeout(
		from *peer.RemoteNode,
		item T,
		timeout time.Duration,
	) (U, bool)
}
