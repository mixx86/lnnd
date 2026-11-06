package sched

import (
	"lnn/internal/queues"
	"sync"
	"sync/atomic"
	"time"
)

type SourceID uint16

const (
	RequestsCacheID SourceID = iota
	RequestsID
	FilesID
)

type DiskLoadState uint8

const (
	DiskLoadNormal DiskLoadState = iota
	DiskLoadElevated
	DiskLoadSaturated
)

type DiskLoadTracker struct {
	/* State is read by every writer, and pollDiskLoad holds diskTrackerMu
		* periodically, so an atomic (instead of using diskTrackerMu) keeps
	    * writers from waiting on it.
	*/
	State        atomic.Uint32
	SmoothedUtil float64
	Initialized  bool
	HighSince    time.Time
	LowSince     time.Time
}

func (d *DiskLoadTracker) GetState() DiskLoadState {
	return DiskLoadState(d.State.Load())
}

func (d *DiskLoadTracker) SetState(state uint32) {
	d.State.Store(state)
}

type Writer struct {
	source         SourceID
	turn           chan struct{}
	requestedBytes int64
	blockedSince   time.Time
	grantedAt      time.Time
	granted        bool
	budget         time.Duration
}

type WriterConfig struct {
	baseWeight float64
	weight     float64

	baseExpectedBytes int64
	expectedBytes     int64

	recentWriteTime  time.Duration
	typicalWriteTime time.Duration
	samples          int
}

type QueuePressure struct {
	Pressured atomic.Bool
	Queue     *queues.QueueConfig
}

type Service struct {
	// Fuck Go for not letting generic Queue[T, U] union cleanly here.
	// These queues have different concrete T/U types, so the queue map has to
	// use any even though the queues themselves are strongly typed.
	queues map[queues.QueueID]any

	sourceQueues  map[SourceID]queues.QueueID
	queuePressure map[queues.QueueID]*QueuePressure

	pressure            map[SourceID]*atomic.Uint32
	pressureChangeCond  *sync.Cond
	pressureReleaseCond *sync.Cond

	writeTurnMu   sync.Mutex
	writeTurnCond *sync.Cond

	// WCB stands for Write Control Block, based on
	// the same jargon used in OS process schedulers.
	WCB map[SourceID]*WriterConfig

	waitingWriters []*Writer
	activeWriters  map[*Writer]struct{}

	maxWriters               int
	maxWritersElevated       int
	maxWritersWhenOverloaded int

	diskDevice    string
	diskLoad      DiskLoadTracker
	diskTrackerMu sync.Mutex

	schedulerWake chan struct{}
}
