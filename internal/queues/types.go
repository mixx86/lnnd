package queues

import (
	"lnn/internal/peer"
	"lnn/internal/protocol"

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
type PendingRequest struct {
	From       *peer.RemoteNode
	Req        protocol.InboundRequest
	Written    chan error
	EnqueuedAt uint64
	Tag        uint16
}

type RequestsQueue struct {
	queue      chan PendingRequest
}

type FilesQueue struct {
}

type Queue[T any, U any] interface {
	Dequeue() U
	TryDequeue() (U, bool)
	TryEnqueue(from *peer.RemoteNode, item T) (U, bool)
	TryEnqueueWithTimeout(from *peer.RemoteNode, item T, timeout time.Duration) (U, bool)
}
