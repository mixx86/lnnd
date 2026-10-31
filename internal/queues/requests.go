package queues

import (
	"math"
	"os"
	"strconv"
	"time"

	"lnn/internal/peer"
	"lnn/internal/protocol"
)

const defaultReqChannelSize uint16 = 4096

func requestsQueueSize() uint16 {
	if s := os.Getenv("LNND_REQ_QUEUE_SIZE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n < math.MaxUint16 {
			return uint16(n)
		}
	}
	return defaultReqChannelSize
}

func NewRequestsQueue() *RequestsQueue {
	return &RequestsQueue{
		queue: make(chan PendingRequest, requestsQueueSize()),
	}
}

func (q *RequestsQueue) makePending(from *peer.RemoteNode, req protocol.InboundRequest, tag uint16) PendingRequest {
	return PendingRequest{
		From:       from,
		Req:        req,
		Written:    make(chan error),
		EnqueuedAt: uint64(time.Now().Unix()),
		Tag: tag,
	}
}

func (q *RequestsQueue) TryEnqueue(from *peer.RemoteNode, req protocol.InboundRequest, tag uint16) (PendingRequest, bool) {
	pr := q.makePending(from, req, tag)
	select {
	case q.queue <- pr:
		return pr, true
	default:
		return PendingRequest{}, false
	}
}

func (q *RequestsQueue) TryEnqueueWithTimeout(from *peer.RemoteNode, req protocol.InboundRequest, tag uint16, timeout time.Duration) (PendingRequest, bool) {
	pr := q.makePending(from, req, tag)
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

