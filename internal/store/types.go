package store

import (
	"lnn/internal/peer"
	"lnn/internal/sched"
	"os"
	"sync"
	"sync/atomic"
)

type RequestsCache struct {
	mu sync.RWMutex

	file *os.File

	entries map[uint32]uint16

	pending    int
	flushEvery int
}

type RecoveryEntry struct {
	Offset   int64
	PeerUUID [16]byte
	ReqID    uint32
}

type RequestsWriter struct {
	canPath string
	sm      *peer.SessionManager
	mu      sync.Mutex

	reqID    atomic.Uint32
	reqCache *RequestsCache
	recovery []RecoveryEntry
}

type Writers struct {
	RequestsWriter
}

func (w *Writers) Run(schedService *sched.Service) {
	w.RequestsWriter.Run(schedService)
}
