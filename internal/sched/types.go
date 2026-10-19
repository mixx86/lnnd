package sched

import (
	"lnn/internal/queues"
	"sync"
	"sync/atomic"
)

type BPSourceID uint16

const (
	BPRequestsCacheID BPSourceID = iota
	BPRequestsID
	BPFilesID
)

type Service struct {
	// Fuck Go for not letting generic Queue[T, U] union cleanly here.
	// These queues have different concrete T/U types, so the queue map has to
	// use any even though the queues themselves are strongly typed.
	queues map[queues.QueueID]any

	pressure map[BPSourceID]*atomic.Uint32

	// DO NOT create more mutexes. pressureChangeMu exists because waiting for a pressure
	// change does not require any validation, it just locks and waits for a signal. Using the
	// same mutex (pressureMu) may add a tiny wait to the whole workflow (when it isn't actually necessary),
	// which could accumulate to a considerable amount of time.
	pressureChangeMu sync.Mutex
	pressureMu       sync.Mutex

	pressureReleaseCond *sync.Cond
	pressureChangeCond  *sync.Cond

	threshold uint64
}
