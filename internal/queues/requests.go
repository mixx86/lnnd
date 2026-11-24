package queues

import (
	"math"
	"os"
	"strconv"
	"time"

	"lnn/internal/config"
	"lnn/internal/peer"
	"lnn/internal/protocol"
)

const defaultReqChannelSize uint16 = 4096

func requestsQueueSize() uint16 {
	if s := os.Getenv("LNND_REQ_QUEUE_SIZE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 &&
			n < math.MaxUint16 {
			return uint16(n)
		}
	}
	return defaultReqChannelSize
}

func NewRequestsQueue() *RequestsQueue {
	q := &RequestsQueue{
		queue: make(chan PendingRequest, requestsQueueSize()),
	}
	q.config.MaxQueuedBytes = config.RequestsQueueMaxBytes()
	return q
}

// Config returns a pointer because QueueConfig holds atomics: a copy would
// silently stop tracking the queue.
func (q *RequestsQueue) Config() *QueueConfig {
	return &q.config
}

func (q *RequestsQueue) makePending(
	remote *peer.RemoteNode,
	req protocol.InboundRequest,
	tag uint32,
	sent bool,
) PendingRequest {
	// Written is buffered for the writer's single result: SubmitRequest
	// stops listening after writeConfirmTimeout, and an unbuffered send
	// would then block the writer, and every request after it, forever.
	return PendingRequest{
		Peer:       remote,
		Req:        req,
		Written:    make(chan WriteResult, 1),
		EnqueuedAt: uint64(time.Now().Unix()),
		Tag:        tag,
		Sent:       sent,
	}
}

func (q *RequestsQueue) TryEnqueue(
	remote *peer.RemoteNode,
	req protocol.InboundRequest,
	tag uint32,
	sent bool,
) (PendingRequest, bool) {
	pr := q.makePending(remote, req, tag, sent)
	select {
	case q.queue <- pr:
		return pr, true
	default:
		return PendingRequest{}, false
	}
}

func (q *RequestsQueue) TryEnqueueWithTimeout(
	remote *peer.RemoteNode,
	req protocol.InboundRequest,
	tag uint32,
	sent bool,
	timeout time.Duration,
) (PendingRequest, bool) {
	pr := q.makePending(remote, req, tag, sent)
	select {
	case q.queue <- pr:
		pr.EnqueuedAt = uint64(time.Now().Unix())
		return pr, true
	case <-time.After(timeout):
		return PendingRequest{}, false
	}
}

func (q *RequestsQueue) Dequeue() PendingRequest {
	return <-q.queue
}

func (q *RequestsQueue) TryDequeue() (PendingRequest, bool) {
	select {
	case req := <-q.queue:
		return req, true
	default:
		return PendingRequest{}, false
	}
}
