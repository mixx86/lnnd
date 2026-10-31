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

	capacity uint32
}

type RequestsFile struct {
	path     string
	file     *os.File
	mu       sync.Mutex
	recovery []RecoveryEntry
}

type RecoveryEntry struct {
	Offset   int64
	PeerUUID [16]byte
	ReqID    uint32
}

var reqID atomic.Uint32

type RequestsWriter struct {
	sm *peer.SessionManager

	reqFile  *RequestsFile
	reqCache *RequestsCache
}

type Writers struct {
	RequestsWriter
}

func (w *Writers) Run(schedService *sched.Service) {
	w.RequestsWriter.Run(schedService)
}
