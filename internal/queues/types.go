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
	From       *peer.RemoteNode
	Req        protocol.InboundRequest
	Written    chan error
	EnqueuedAt uint64
	Tag        uint32
	// The peer's id for the request, 0 when the request is ours.
	RMTReqID uint32
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
