package store

import (
	"lnn/internal/logger"

	"lnn/internal/peer"

	"lnn/internal/queues"
	"lnn/internal/sched"

	"os"
	"path/filepath"
)

const (
	maxBatch        = 64
	checkpointAfter = 512
)

type OP_KIND uint8

const (
	OP_REQUEST OP_KIND = iota
	OP_ACCEPTED
	OP_REJECTED
	OP_CHECKPOINT
)

const (
	CACHE_REQ_ID_LEN = 4
	CACHE_TAG_LEN    = 2
	CACHE_ENTRY_LEN  = CACHE_REQ_ID_LEN + CACHE_TAG_LEN
)

const (
	F_HEADER_MAGIC      uint32 = 0x4C4E4E52 // lnnr
	F_HEADER_MAGIC_LEN         = 4
	F_HEADER_LCO_LEN           = 8
	F_HEADER_REQ_ID_LEN        = 4

	F_HEADER_TOTAL_LEN = F_HEADER_MAGIC_LEN + F_HEADER_LCO_LEN + F_HEADER_REQ_ID_LEN
)

const (
	OP_KIND_LEN      = 1
	ENTRY_LENGTH_LEN = 4
	ENTRY_HEADER_LEN = ENTRY_LENGTH_LEN + OP_KIND_LEN
)

const (
	CHECKPOINT_CSUM_LEN = 4
	CHECKPOINT_LEN      = ENTRY_HEADER_LEN + CHECKPOINT_CSUM_LEN
)

const (
	REQUEST_REQ_TYPE_LEN  = 2
	REQUEST_PEER_UUID_LEN = 16
	REQUEST_REQ_ID_LEN    = 4
	REQUEST_UNIX_TS_LEN   = 8

	REQUEST_PEER_UUID_OFFSET = ENTRY_HEADER_LEN + REQUEST_REQ_TYPE_LEN
	REQUEST_REQ_ID_OFFSET    = REQUEST_PEER_UUID_OFFSET + REQUEST_PEER_UUID_LEN

	REQUEST_RECOVERY_MIN_LEN = ENTRY_HEADER_LEN +
		REQUEST_REQ_TYPE_LEN +
		REQUEST_PEER_UUID_LEN +
		REQUEST_REQ_ID_LEN
)

func NewRequestsWriter(dir string, sm *peer.SessionManager) *RequestsWriter {
	reqCache, err := OpenRequestsCache(
		filepath.Join(dir, "requests.cache"),
		300,
	)
	if err != nil {
		logger.Daemon.Fatalf("error opening requests cache: %v", err)
	}
	return &RequestsWriter{
		canPath:  filepath.Join(dir, "requests"),
		sm:       sm,
		reqCache: reqCache,
	}
}

func (w *RequestsWriter) Run(s *sched.Service) {
	info, err := os.Stat(w.canPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.writeRequestsFileHeader()
		} else {
			logger.Daemon.Fatalf("error: %v", err)
		}
	}

	if info.IsDir() {
		logger.Daemon.Fatalf("error: requests shouldn't be a directory at %v", w.canPath)
	}

	if info.Size() == 0 {
		w.writeRequestsFileHeader()
	}

	f, err := os.OpenFile(w.canPath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		logger.Daemon.Fatalf("error: something went wrong while opening the requests file at: %v: %v", w.canPath, err)
	}
	defer f.Close()

	if !w.checkRequestsFileHeader(f) {
		w.writeRequestsFileHeader()
	}

	if err, recoverable := w.checkEntries(f, info); err != nil {
		if recoverable {
			if err = w.recoverEntries(f); err != nil {
				logger.Daemon.Printf("couldn't recover requests: %v", err)
			}
		} else {
			w.removeTail(f, info)
		}
	}

	for {
		req, err := s.NextRequest()
		if err != nil {
			logger.Daemon.Fatalf("error: %v", err)
		}
		batch := []queues.PendingRequest{req}

		for len(batch) < maxBatch {
			req, ok, err := s.TryNextRequest()
			if err != nil {
				logger.Daemon.Fatalf("error: %v", err)
			}

			if !ok {
				break
			}
			batch = append(batch, req)
		}

		w.persistRequests(batch, s)
	}
}

func (w *RequestsWriter) nextReqID() uint32 {
	return w.reqID.Add(1) - 1
}

