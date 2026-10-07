package store

import (
	"encoding/binary"
	"lnn/internal/logger"

	"lnn/internal/peer"

	"lnn/internal/queues"
	"lnn/internal/sched"

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

	REQUEST_STATIC_FIELDS_LEN = REQUEST_REQ_TYPE_LEN + REQUEST_PEER_UUID_LEN + REQUEST_REQ_ID_LEN + REQUEST_UNIX_TS_LEN

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

	reqFile, err := OpenRequestsFile(filepath.Join(dir, "requests"))
	if err != nil {
		logger.Daemon.Fatalf("error opening the requests file: %v", err)
	}

	return &RequestsWriter {
		sm: sm,
		reqFile: reqFile,
		reqCache: reqCache,
	}
}

func (w *RequestsWriter) Run(s *sched.Service) {
	srcReqID, err := w.reqFile.getReqID()
	if err != nil {
		logger.Daemon.Fatalf("error: %v", err)
	}
	reqID.Store(srcReqID)

	if err, recoverable := w.reqFile.checkEntries(); err != nil {
		if recoverable {
			if err = w.recoverEntries(); err != nil {
				logger.Daemon.Printf("couldn't recover requests: %v", err)
			}
		} else {
			err := w.reqFile.removeTail()
			if err != nil {
				logger.Daemon.Fatalf("error: %v", err)
			}
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

func (r *RequestsFile) getReqID() (uint32, error) {
	var buf [F_HEADER_REQ_ID_LEN]byte

	if _, err := r.file.ReadAt(buf[:], F_HEADER_MAGIC_LEN+F_HEADER_LCO_LEN); err != nil {
		logger.Daemon.Printf("error: reading current_req_id: %v", err)
		return 0, err
	}

	return binary.BigEndian.Uint32(buf[:]), nil
}

func (r *RequestsFile) nextReqID() uint32 {
	id := reqID.Add(1)

	var buf [F_HEADER_REQ_ID_LEN]byte
	binary.BigEndian.PutUint32(buf[:], id)

	if _, err := r.file.WriteAt(buf[:], F_HEADER_MAGIC_LEN+F_HEADER_LCO_LEN); err != nil {
		logger.Daemon.Printf("error: writing current_req_id: %v", err)
	}

	return id
}
