// Package iocore owns the inbound request queue and the io_uring pressure signal.
// Future: io_uring scheduler and submission/completion machinery lives here.
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

// New creates and initialises an iocore Manager.
func NewRequestsQueue() *RequestsQueue {
	return &RequestsQueue{
		queue: make(chan PendingRequest, requestsQueueSize()),
	}
}

func (q *RequestsQueue) makePending(from *peer.RemoteNode, req protocol.InboundRequest) PendingRequest {
	return PendingRequest{
		ID:         q.reqCounter.Add(1),
		From:       from,
		Req:        req,
		Written:    make(chan error),
		EnqueuedAt: time.Now(),
	}
}

func (q *RequestsQueue) TryEnqueue(from *peer.RemoteNode, req protocol.InboundRequest) (PendingRequest, bool) {
	pr := q.makePending(from, req)
	select {
	case q.queue <- pr:
		return pr, true
	default:
		return PendingRequest{}, false
	}
}

func (q *RequestsQueue) TryEnqueueWithTimeout(from *peer.RemoteNode, req protocol.InboundRequest, timeout time.Duration) (PendingRequest, bool) {
	pr := q.makePending(from, req)
	ptr := &pr
	select {
	case q.queue <- pr:
		ptr.EnqueuedAt = time.Now()
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

/*// AddPressure signals an io_uring operation started. Must pair with RemovePressure.
func (m *Manager) AddPressure() {
	m.ioPressure.Add(1)
}

// RemovePressure signals an io_uring operation completed.
func (m *Manager) RemovePressure() {
	m.ioPressure.Add(-1)
}

// Pressure returns the current number of active io_uring operations.
func (m *Manager) Pressure() int32 {
	return m.ioPressure.Load()
}

func (m *Manager) PressureDone() <-chan struct{} {
	return m.pressureDone
}*/
