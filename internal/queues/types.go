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

// PendingRequest is transient: it exists only while the WAL writer is persisting
// the request to disk. Once Written is closed, this struct must not be retained —
// the on-disk file is the sole source of truth for pending requests.
// From is needed by the WAL writer to record the peer UUID.
type PendingRequest struct {
	ID         uint32
	From       *peer.RemoteNode
	Req        protocol.InboundRequest
	Written    chan error
	EnqueuedAt time.Time
}

type RequestsQueue struct {
	queue      chan PendingRequest
	reqCounter atomic.Uint32
}

type Queue[T any, U any] interface {
	Dequeue() U
	TryDequeue() (U, bool)
	TryEnqueue(from *peer.RemoteNode, item T) (U, bool)
	TryEnqueueWithTimeout(from *peer.RemoteNode, item T, timeout time.Duration) (U, bool)
}
