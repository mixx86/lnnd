package store

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"lnn/internal/logger"
	"lnn/internal/network"
	"lnn/internal/peer"
	"lnn/internal/protocol"
	"lnn/internal/queues"
	"lnn/internal/sched"

	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// File header format:
// [magic:4][lco:8][next_req_id:4]
// lco stands for "last checkpoint offset"
// Entry format (one per line):
// Entry header:
//
// [entry_length:4][op:1]
//
// Payload:
//
// Request (op=0):
// [req_type:2][peer_uuid:16][req_id:4][unix_ts:8]
// [src_len:2][src:src_len][dst_len:2][dst:dst_len]
//
// Accepted (op=1):
// [req_id:4]
//
// Rejected (op=2):
// [req_id:4]
//
// Checkpoint (op=3):
// [checksum:4]

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

// writeRequestsFileHeader assumes the file is empty; thus, it creates a first
// checkpoint, which is not part of the file header.
func (w *RequestsWriter) writeRequestsFileHeader() {
	f, err := os.OpenFile(w.canPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		logger.Daemon.Fatalf("error: something went wrong while opening the requests file at: %v: %v", w.canPath, err)
	}
	defer f.Close()

	var header [F_HEADER_TOTAL_LEN]byte
	binary.BigEndian.PutUint32(header[:F_HEADER_MAGIC_LEN], F_HEADER_MAGIC)
	binary.BigEndian.PutUint64(header[F_HEADER_MAGIC_LEN:F_HEADER_TOTAL_LEN], F_HEADER_TOTAL_LEN)

	var checkpoint [9]byte
	binary.BigEndian.PutUint32(checkpoint[:ENTRY_LENGTH_LEN], CHECKPOINT_LEN)
	binary.BigEndian.PutUint32(checkpoint[ENTRY_LENGTH_LEN:ENTRY_LENGTH_LEN+OP_KIND_LEN], uint32(OP_CHECKPOINT))
	binary.BigEndian.PutUint32(checkpoint[ENTRY_HEADER_LEN:ENTRY_HEADER_LEN+CHECKPOINT_CSUM_LEN], uint32(0))

	if _, err := f.Write(header[:]); err != nil {
		logger.Daemon.Fatalf("error writing the file header: %v", err)
	}
	if _, err := f.Write(checkpoint[:]); err != nil {
		logger.Daemon.Fatalf("error writing the initial checkpoint: %v", err)
	}

	if err := f.Sync(); err != nil {
		logger.Daemon.Fatalf("error writing the file header: %v", err)
	}
}

func (w *RequestsWriter) checkRequestsFileHeader(f *os.File) bool {
	header := make([]byte, F_HEADER_TOTAL_LEN)
	_, err := f.Read(header)
	if err != nil {
		logger.Daemon.Fatalf("error: something went wrong while reading the requests file at: %v: %v", w.canPath, err)
	}

	magic := binary.BigEndian.Uint32(header[:F_HEADER_MAGIC_LEN])
	if magic != F_HEADER_MAGIC {
		return false
	}

	lco := binary.BigEndian.Uint64(header[F_HEADER_MAGIC_LEN : F_HEADER_MAGIC_LEN+F_HEADER_LCO_LEN])

	info, err := f.Stat()
	if err != nil {
		logger.Daemon.Fatalf(
			"error: couldn't stat requests file at %v: %v",
			w.canPath,
			err,
		)
	}

	if lco < F_HEADER_TOTAL_LEN || lco >= uint64(info.Size()) {
		return false
	}

	return true
}

func (w *RequestsWriter) checkEntries(f *os.File, info os.FileInfo) (error, bool) {
	lco, err := w.getLco(f)
	if err != nil {
		return err, false
	}

	fileSize := info.Size()
	checkpointEnd := int64(lco) + int64(CHECKPOINT_LEN)

	if int64(lco) < int64(F_HEADER_TOTAL_LEN) || checkpointEnd > fileSize {
		return fmt.Errorf("invalid last checkpoint offset: %d", lco), false
	}

	// There are entries after the last checkpoint. Validate only the
	// uncheckpointed tail using entry_length.
	if checkpointEnd < fileSize {
		return w.checkTailEntries(f, checkpointEnd, fileSize)
	}

	// The last entry is the last checkpoint. Find the previous
	// checkpoint so we can validate only the region it closes.
	previousCheckpoint, err := w.findPreviousCheckpoint(f, lco)
	if err != nil {
		return err, false
	}

	offset := previousCheckpoint + int64(CHECKPOINT_LEN)
	var checksum uint32
	var recoverable bool

	for offset < int64(lco) {
		entry, entryLength, op, err := readEntry(f, offset, lco)
		if err != nil {
			return err, recoverable
		}

		if op == OP_REQUEST &&
			entryLength >= uint32(REQUEST_RECOVERY_MIN_LEN) {
			w.recovery = append(w.recovery, RecoveryEntry{
				Offset: offset,
				PeerUUID: uuid.UUID(
					entry[REQUEST_PEER_UUID_OFFSET : REQUEST_PEER_UUID_OFFSET+REQUEST_PEER_UUID_LEN],
				),
				ReqID: binary.BigEndian.Uint32(
					entry[REQUEST_REQ_ID_OFFSET : REQUEST_REQ_ID_OFFSET+REQUEST_REQ_ID_LEN],
				),
			})
			recoverable = true
		}

		checksum += calculateCheckSum(entry)
		offset += int64(entryLength)
	}

	if offset != int64(lco) {
		return fmt.Errorf(
			"entries do not align with last checkpoint at offset %d",
			lco,
		), recoverable
	}

	var checksumBuf [CHECKPOINT_CSUM_LEN]byte

	if _, err := f.ReadAt(
		checksumBuf[:],
		int64(lco)+int64(ENTRY_HEADER_LEN),
	); err != nil {
		return err, recoverable
	}

	storedChecksum := binary.BigEndian.Uint32(checksumBuf[:])

	if checksum != storedChecksum {
		return fmt.Errorf(
			"checksum mismatch at checkpoint %d: got %d, want %d",
			lco,
			checksum,
			storedChecksum,
		), recoverable
	}

	return nil, false
}

func (w *RequestsWriter) checkTailEntries(
	f *os.File,
	start int64,
	fileSize int64,
) (error, bool) {
	offset := start

	for offset < fileSize {
		entry, entryLength, op, err := readEntry(f, offset, fileSize)
		if err != nil {
			// The entry is truncated/corrupt. Check whether enough of the
			// request survived to recover peer_uuid + req_id.
			remaining := fileSize - offset
			minRequestLen := int64(REQUEST_RECOVERY_MIN_LEN)

			if remaining >= minRequestLen {
				var opBuf [OP_KIND_LEN]byte

				if _, readErr := f.ReadAt(
					opBuf[:],
					offset+int64(ENTRY_LENGTH_LEN),
				); readErr != nil {
					return err, false
				}

				if OP_KIND(opBuf[0]) == OP_REQUEST {
					return err, true
				}
			}

			return err, false
		}

		// A checkpoint after lco would mean the file header's lco is stale.
		// It should not happen if lco is maintained correctly.
		if op == OP_CHECKPOINT {
			return fmt.Errorf(
				"unexpected checkpoint at offset %d after last checkpoint",
				offset,
			), false
		}

		// entry is intentionally kept here because readEntry verifies that
		// the complete entry fits in the file.
		_ = entry

		offset += int64(entryLength)
	}

	if offset != fileSize {
		return fmt.Errorf("entries do not align with end of file"), false
	}

	return nil, false
}

func (w *RequestsWriter) findPreviousCheckpoint(
	f *os.File,
	lco int64,
) (int64, error) {
	offset := int64(F_HEADER_TOTAL_LEN)
	previousCheckpoint := int64(-1)

	for offset < lco {
		_, entryLength, op, err := readEntry(f, offset, lco)
		if err != nil {
			return 0, err
		}

		entryEnd := offset + int64(entryLength)

		if entryEnd > lco {
			return 0, fmt.Errorf(
				"entry at offset %d overlaps last checkpoint at %d",
				offset,
				lco,
			)
		}

		if op == OP_CHECKPOINT {
			if entryLength != uint32(CHECKPOINT_LEN) {
				return 0, fmt.Errorf(
					"invalid checkpoint length %d at offset %d",
					entryLength,
					offset,
				)
			}

			previousCheckpoint = offset
		}

		offset = entryEnd
	}

	if offset != lco {
		return 0, fmt.Errorf(
			"could not align with last checkpoint at offset %d",
			lco,
		)
	}

	if previousCheckpoint == -1 {
		return 0, fmt.Errorf(
			"%w before last checkpoint at %d",
			ErrNoPreviousCheckpoint,
			lco,
		)
	}

	return previousCheckpoint, nil
}

func readEntry(
	f *os.File,
	offset int64,
	limit int64,
) ([]byte, uint32, OP_KIND, error) {
	if offset < 0 || offset >= limit {
		return nil, 0, 0, fmt.Errorf(
			"invalid entry offset %d",
			offset,
		)
	}

	if limit-offset < int64(ENTRY_LENGTH_LEN) {
		return nil, 0, 0, fmt.Errorf(
			"%w at offset %d",
			ErrTruncatedEntry,
			offset,
		)
	}

	var lengthBuf [ENTRY_LENGTH_LEN]byte
	if _, err := f.ReadAt(lengthBuf[:], offset); err != nil {
		return nil, 0, 0, fmt.Errorf(
			"reading entry length at offset %d: %w",
			offset,
			err,
		)
	}

	entryLength := binary.BigEndian.Uint32(lengthBuf[:])

	if entryLength < uint32(ENTRY_HEADER_LEN) {
		return nil, 0, 0, fmt.Errorf(
			"%w at offset %d: invalid length %d",
			ErrCorruptEntry,
			offset,
			entryLength,
		)
	}

	entryEnd := offset + int64(entryLength)

	if entryEnd > limit {
		return nil, entryLength, 0, fmt.Errorf(
			"%w at offset %d: length %d exceeds limit",
			ErrTruncatedEntry,
			offset,
			entryLength,
		)
	}

	entry := make([]byte, entryLength)
	if _, err := f.ReadAt(entry, offset); err != nil {
		return nil, entryLength, 0, fmt.Errorf(
			"reading entry at offset %d: %w",
			offset,
			err,
		)
	}

	op := OP_KIND(entry[ENTRY_LENGTH_LEN])

	switch op {
	case OP_REQUEST, OP_ACCEPTED, OP_REJECTED, OP_CHECKPOINT:
	default:
		return nil, entryLength, op, fmt.Errorf(
			"%w at offset %d: operation %d",
			ErrCorruptEntry,
			offset,
			op,
		)
	}

	return entry, entryLength, op, nil
}

func (w *RequestsWriter) recoverEntries(f *os.File) error {
	for _, entry := range w.recovery {
		tag, ok := w.reqCache.GetTag(entry.ReqID)
		if !ok {
			continue
		}

		if err := w.recoverRequest(
			entry.PeerUUID,
			entry.ReqID,
			tag,
		); err != nil {
			return fmt.Errorf(
				"recovering request %d at offset %d: %w",
				entry.ReqID,
				entry.Offset,
				err,
			)
		}
	}

	return nil
}

func (w *RequestsWriter) recoverRequest(
	peerUUID [16]byte,
	reqID uint32,
	tag uint16,
) error {
	s, ok := w.sm.Get(peerUUID)
	if !ok {
		return fmt.Errorf("no peer with uuid %v was found", peerUUID)
	}
	network.SendTCP(s, protocol.Frame{
		Body: protocol.Error{
			Code:    protocol.Recov,
			Message: "",
		},
	}.WithTag(tag))

	return nil
}

func (w *RequestsWriter) writeRecoveredRequest(
	f *os.File,
	request []byte,
) error {
	if len(request) < ENTRY_HEADER_LEN {
		return fmt.Errorf("invalid recovered request length %d", len(request))
	}

	entryLength := binary.BigEndian.Uint32(
		request[:ENTRY_LENGTH_LEN],
	)

	if int(entryLength) != len(request) {
		return fmt.Errorf(
			"recovered request length mismatch: header=%d actual=%d",
			entryLength,
			len(request),
		)
	}

	if OP_KIND(request[ENTRY_LENGTH_LEN]) != OP_REQUEST {
		return fmt.Errorf(
			"recovered entry is not a request: op=%d",
			request[ENTRY_LENGTH_LEN],
		)
	}

	_, err := f.Write(request)
	return err
}

func (w *RequestsWriter) removeTail(f *os.File, info os.FileInfo) error {
	lco, err := w.getLco(f)
	if err != nil {
		return err
	}

	checkpointEnd := int64(lco) + int64(CHECKPOINT_LEN)

	if checkpointEnd > info.Size() {
		return fmt.Errorf(
			"invalid last checkpoint offset %d",
			lco,
		)
	}

	// The last checkpoint is valid, so everything after it is tail.
	if checkpointEnd < info.Size() {
		if err := f.Truncate(checkpointEnd); err != nil {
			return fmt.Errorf(
				"truncating requests tail at %d: %w",
				checkpointEnd,
				err,
			)
		}

		return f.Sync()
	}

	// The last entry is the checkpoint itself. If we got here because
	// checkEntries found corruption, the checkpoint checksum is invalid.
	previousCheckpoint, err := w.findPreviousCheckpoint(f, int64(lco))
	if err != nil {
		// The initial checkpoint is the only checkpoint and there is
		// therefore nothing before it to recover.
		if errors.Is(err, ErrNoPreviousCheckpoint) {
			if err := f.Truncate(F_HEADER_TOTAL_LEN + CHECKPOINT_LEN); err != nil {
				return err
			}

			return f.Sync()
		}

		return err
	}

	previousCheckpointEnd :=
		previousCheckpoint + int64(CHECKPOINT_LEN)

	if err := f.Truncate(previousCheckpointEnd); err != nil {
		return fmt.Errorf(
			"truncating corrupted checkpoint at %d: %w",
			lco,
			err,
		)
	}

	if err := w.setLco(f, uint64(previousCheckpoint)); err != nil {
		return err
	}

	return f.Sync()
}

func (w *RequestsWriter) persistRequests(batch []queues.PendingRequest, s *sched.Service) error {
	f, err := os.OpenFile(w.canPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)

	if err := w.writeBatch(f, bw, batch, OP_REQUEST); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	for s.IsBackPressured(sched.BPRequestsID) {
		if len(batch) >= maxBatch {
			s.WaitForPressureRelease(sched.BPRequestsID)
			break
		}

		req, err := s.NextRequest()
		if err != nil {
			notifyWriteResult(batch, err)
			return err
		}

		batch = append(batch, req)

		if err := w.writeBatch(f, bw, []queues.PendingRequest{req}, OP_REQUEST); err != nil {
			notifyWriteResult(batch, err)
			return err
		}
	}

	if err = bw.Flush(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}
	if err = f.Sync(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	if err := w.checkpoint(f, bw); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	if err := bw.Flush(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}
	if err := f.Sync(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	notifyWriteResult(batch, nil)
	return nil
}

// WARNING: writeBatch does ONLY write in the buffer, make sure to call bw.Flush and f.Sync
// once you're done writing unless you don't actually want to save the content.
func (w *RequestsWriter) writeBatch(f *os.File, bw *bufio.Writer, batch []queues.PendingRequest, op OP_KIND) error {
	for _, pr := range batch {
		var fields []byte

		switch op {
		case OP_REQUEST:
			fields = pr.Req.Encode()

		case OP_CHECKPOINT:
			const maxRetries = 5

			var info os.FileInfo
			var err error

			for attempt := 1; attempt <= maxRetries; attempt++ {
				info, err = f.Stat()
				if err == nil {
					break
				}

				logger.Daemon.Printf(
					"stat failed (attempt %d/%d): %v",
					attempt,
					maxRetries,
					err,
				)

				time.Sleep(time.Second)
			}

			if err != nil {
				logger.Daemon.Printf("could not stat file after %d attempts: %v", maxRetries, err)
				return err
			}

			lco, err := w.getLco(f)
			if err != nil {
				logger.Daemon.Fatalf("error: %v", err)
			}

			checkpointEnd := lco + CHECKPOINT_LEN

			switch {
			case checkpointEnd == info.Size():
				return fmt.Errorf("cannot put a new checkpoint without new entries")
			case checkpointEnd > info.Size():
				return fmt.Errorf("invalid last checkpoint offset")
			}

			if _, err := f.Seek(int64(checkpointEnd), io.SeekStart); err != nil {
				return err
			}

			var checksum uint32
			var entryLengthBuf [ENTRY_LENGTH_LEN]byte

			for {
				entryOffset, err := f.Seek(0, io.SeekCurrent)
				if err != nil {
					return err
				}

				if entryOffset >= info.Size() {
					break
				}

				if int64(ENTRY_LENGTH_LEN) > info.Size()-entryOffset {
					return fmt.Errorf("truncated entry length at offset %d", entryOffset)
				}

				if _, err := io.ReadFull(f, entryLengthBuf[:]); err != nil {
					return fmt.Errorf("reading entry length at offset %d: %w", entryOffset, err)
				}

				entryLength := binary.BigEndian.Uint32(entryLengthBuf[:])

				if entryLength < uint32(ENTRY_HEADER_LEN) {
					return fmt.Errorf(
						"invalid entry length %d at offset %d",
						entryLength,
						entryOffset,
					)
				}

				remaining := info.Size() - entryOffset

				if uint64(entryLength) > uint64(remaining) {
					return fmt.Errorf(
						"truncated entry at offset %d: length=%d, remaining=%d",
						entryOffset,
						entryLength,
						remaining,
					)
				}

				entry := make([]byte, entryLength)
				copy(entry[:ENTRY_LENGTH_LEN], entryLengthBuf[:])

				if _, err := io.ReadFull(f, entry[ENTRY_LENGTH_LEN:]); err != nil {
					return fmt.Errorf("reading entry at offset %d: %w", entryOffset, err)
				}

				checksum += calculateCheckSum(entry)
			}

			fields = make([]byte, CHECKPOINT_CSUM_LEN)
			binary.BigEndian.PutUint32(fields, checksum)

		case OP_ACCEPTED, OP_REJECTED:
			fields = nil

		default:
			return fmt.Errorf("unknown operation kind: %d", op)
		}

		entryLength := uint32(ENTRY_HEADER_LEN + len(fields))
		header := encodeEntryHeader(op, entryLength)

		req := make([]byte, 0, entryLength)
		req = append(req, header[:]...)
		req = append(req, fields...)
		if _, err := bw.Write(req); err != nil {
			return err
		}
	}

	return nil
}

func (w *RequestsWriter) checkpoint(f *os.File, bw *bufio.Writer) error {
	return w.writeBatch(f, bw, nil, OP_CHECKPOINT)
}

func notifyWriteResult(batch []queues.PendingRequest, err error) {
	for _, pr := range batch {
		pr.Written <- err
	}
}

func (w *RequestsWriter) setLco(f *os.File, lco uint64) error {
	var buf [F_HEADER_LCO_LEN]byte

	binary.BigEndian.PutUint64(buf[:], lco)

	if _, err := f.WriteAt(
		buf[:],
		F_HEADER_MAGIC_LEN,
	); err != nil {
		return fmt.Errorf("writing lco: %w", err)
	}

	return nil
}

func (w *RequestsWriter) getLco(f *os.File) (int64, error) {
	var b [F_HEADER_LCO_LEN]byte

	if _, err := f.ReadAt(b[:], F_HEADER_MAGIC_LEN); err != nil {
		return 0, err
	}

	return int64(binary.BigEndian.Uint64(b[:])), nil
}

func encodeEntryHeader(op OP_KIND, entryLength uint32) [ENTRY_HEADER_LEN]byte {
	var data [ENTRY_HEADER_LEN]byte

	binary.BigEndian.PutUint32(
		data[:ENTRY_LENGTH_LEN],
		entryLength,
	)
	data[ENTRY_LENGTH_LEN] = uint8(op)

	return data
}

func calculateCheckSum(data []byte) uint32 {
	var checkSum uint32
	for _, b := range data {
		checkSum += uint32(b)
	}
	return checkSum
}
