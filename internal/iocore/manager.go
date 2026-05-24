// Package iocore owns the inbound request queue and the io_uring pressure signal.
// Future: io_uring scheduler and submission/completion machinery lives here.
package iocore

import (
	"os"
	"strconv"
	"time"

	"lnn/internal/peer"
	"lnn/internal/protocol"
)

const defaultReqChannelSize = 4096

func reqChannelSize() int {
	if s := os.Getenv("LNND_REQ_CHANNEL_SIZE"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return defaultReqChannelSize
}


// New creates and initialises an iocore Manager.
func New() *Manager {
	return &Manager{
		reqCh: make(chan PendingRequest, reqChannelSize()),
	}
}

func (m *Manager) makePending(from *peer.RemoteNode, req protocol.InboundRequest) PendingRequest {
	return PendingRequest{
		ID:      m.reqCounter.Add(1),
		From:    from,
		Req:     req,
		Written: make(chan struct{}),
	}
}

// TryEnqueue attempts a non-blocking send. Returns (pr, true) if enqueued, (zero, false) if the channel is full.
func (m *Manager) TryEnqueue(from *peer.RemoteNode, req protocol.InboundRequest) (PendingRequest, bool) {
	pr := m.makePending(from, req)
	select {
	case m.reqCh <- pr:
		return pr, true
	default:
		return PendingRequest{}, false
	}
}

// Enqueue waits up to timeout for space in the channel. Returns (pr, true) if enqueued, (zero, false) on timeout.
func (m *Manager) Enqueue(from *peer.RemoteNode, req protocol.InboundRequest, timeout time.Duration) (PendingRequest, bool) {
	pr := m.makePending(from, req)
	select {
	case m.reqCh <- pr:
		return pr, true
	case <-time.After(timeout):
		return PendingRequest{}, false
	}
}

// ReqCh exposes the read end for the WAL writer to drain.
func (m *Manager) ReqCh() <-chan PendingRequest {
	return m.reqCh
}

// AddPressure signals an io_uring operation started. Must pair with RemovePressure.
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
}
