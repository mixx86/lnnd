package store

import (
	"encoding/binary"
	"fmt"
	"lnn/internal/logger"

	"lnn/internal/queues"
	"lnn/internal/recovery"
	"lnn/internal/sched"

	"path/filepath"
)

const (
	maxBatch        = 64
	checkpointAfter = 512
)

type opKind uint8

const (
	opRequest opKind = iota
	opAccepted
	opRejected
	opCheckpoint
	opSentRequest
)

const (
	fileHeaderMagic    uint32 = 0x4C4E4E52 // "LNNR"
	fileHeaderMagicLen        = 4
	fileHeaderLCOLen          = 8
	fileHeaderReqIDLen        = 4

	fileHeaderLen = fileHeaderMagicLen + fileHeaderLCOLen + fileHeaderReqIDLen
)

const (
	opKindLen      = 1
	entryLengthLen = 4
	entryHeaderLen = entryLengthLen + opKindLen
)

const (
	checkpointChecksumLen = 4
	checkpointLen         = entryHeaderLen + checkpointChecksumLen
)

const (
	requestTypeLen     = 2
	requestPeerUUIDLen = 16
	requestIDLen       = 4
	requestRMTReqIDLen = 4
	requestUnixTSLen   = 8

	requestStaticFieldsLen = requestTypeLen + requestPeerUUIDLen +
		requestIDLen + requestRMTReqIDLen + requestUnixTSLen

	requestPeerUUIDOffset = entryHeaderLen + requestTypeLen
	requestIDOffset       = requestPeerUUIDOffset + requestPeerUUIDLen

	requestRecoveryMinLen = entryHeaderLen +
		requestTypeLen +
		requestPeerUUIDLen +
		requestIDLen
)

func NewRequestsWriter(
	dir string,
	pending *recovery.Pending,
) *RequestsWriter {
	reqCache, err := OpenRequestsCache(
		filepath.Join(dir, "requests.cache"),
		2,
	)
	if err != nil {
		logger.Daemon.Fatalf("error opening requests cache: %v", err)
	}

	reqFile, err := OpenRequestsFile(filepath.Join(dir, "requests"))
	if err != nil {
		logger.Daemon.Fatalf("error opening the requests file: %v", err)
	}

	return &RequestsWriter{
		pending:  pending,
		reqFile:  reqFile,
		reqCache: reqCache,
	}
}

func (w *RequestsWriter) Run(s *sched.Service) {
	srcReqID, err := w.reqFile.getReqID()
	if err != nil {
		logger.Daemon.Fatalf("error: %v", err)
	}
	reqID.Store(srcReqID)

	if err := w.repairOnStartup(); err != nil {
		logger.Daemon.Fatalf("repairing requests file: %v", err)
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

func (w *RequestsWriter) repairOnStartup() error {
	err, recoverable := w.reqFile.checkEntries()
	if err == nil {
		return nil
	}
	logger.Daemon.Printf("requests file damaged: %v", err)

	if recoverable {
		// Truncating without recording would lose the requests for good.
		if err := w.recordRecoveries(); err != nil {
			return fmt.Errorf("recording recoveries: %w", err)
		}
	}
	return w.reqFile.removeTail()
}

func (r *RequestsFile) getReqID() (uint32, error) {
	var buf [fileHeaderReqIDLen]byte

	_, err := r.file.ReadAt(buf[:], fileHeaderMagicLen+fileHeaderLCOLen)
	if err != nil {
		logger.Daemon.Printf("error: reading current_req_id: %v", err)
		return 0, err
	}

	return binary.BigEndian.Uint32(buf[:]), nil
}

func (r *RequestsFile) nextReqID() uint32 {
	id := reqID.Add(1)

	var buf [fileHeaderReqIDLen]byte
	binary.BigEndian.PutUint32(buf[:], id)

	_, err := r.file.WriteAt(buf[:], fileHeaderMagicLen+fileHeaderLCOLen)
	if err != nil {
		logger.Daemon.Printf("error: writing current_req_id: %v", err)
	}

	return id
}
