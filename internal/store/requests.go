package store

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"lnn/internal/logger"
	"lnn/internal/queues"
	"lnn/internal/sched"

	"os"
	"path/filepath"
	"time"
)

// Cosas a añadir:
// HEADER al inicio del archivo que indique offset de archivo hasta checkpoint
// length para cada campo
// checksum al lado del checkpoint
// checkpoint after drenar batch
// al arrancar validar checksum si no hay entrys delante
// separar entry header de payload header

// File header format:
// [magic:4][lco:8]
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
	F_HEADER_MAGIC     uint32 = 0x4C4E4E52 // lnnr
	F_HEADER_MAGIC_LEN        = 4
	F_HEADER_LCO_LEN          = 8
	F_HEADER_TOTAL_LEN        = F_HEADER_MAGIC_LEN + F_HEADER_LCO_LEN
)

const (
	OP_KIND_LEN = 1
	ENTRY_LENGTH_LEN = 4
	ENTRY_HEADER_LEN = ENTRY_LENGTH_LEN + OP_KIND_LEN
	CHECKPOINT_CSUM_LEN = 4
	CHECKPOINT_LEN = ENTRY_HEADER_LEN + CHECKPOINT_CSUM_LEN 
)

func NewRequestsWriter(dir string) *RequestsWriter {
	return &RequestsWriter{
		canPath: filepath.Join(dir, "requests"),
	}
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
	if err := f.Sync(); err != nil {
		logger.Daemon.Fatalf("error writing the file header: %v", err)
	}
}
func (w *RequestsWriter) checkRequestsFileHeader() bool {
	f, err := os.OpenFile(w.canPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		logger.Daemon.Fatalf("error: something went wrong while opening the requests file at: %v: %v", w.canPath, err)
	}
	defer f.Close()

	header := make([]byte, F_HEADER_TOTAL_LEN)
	_, err = f.Read(header)
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

	if !w.checkRequestsFileHeader() {
		w.writeRequestsFileHeader()
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

func encodeEntryHeader(op OP_KIND, entryLength uint32) [5]byte {
    var data [ENTRY_HEADER_LEN]byte

    binary.BigEndian.PutUint32(data[:4], entryLength)
    data[4] = uint8(op)

    return data
}

func (w *RequestsWriter) getLco(f *os.File) (int64, error) {
	if _, err := f.Seek(4, io.SeekStart); err != nil {
		return 0, err
	}
	b := make([]byte, F_HEADER_LCO_LEN)
	if _, err := io.ReadFull(f, b); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b)), nil;
}

func calculateCheckSum(data []byte) uint32 {
	var checkSum uint32
	for _, b := range data {
		checkSum += uint32(b)
	}
	return checkSum
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

