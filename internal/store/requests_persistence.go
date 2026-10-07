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
	"time"
)

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

	lco, err := w.checkpoint(f, bw);
	if err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	if err := w.setLco(f, lco); err != nil {
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

func (w *RequestsWriter) checkpoint(f *os.File, bw *bufio.Writer) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}

	lco := uint64(info.Size())

	if err := w.writeBatch(f, bw, nil, OP_CHECKPOINT); err != nil {
		return 0, err
	}

	return lco, nil
}

func notifyWriteResult(batch []queues.PendingRequest, err error) {
	for _, pr := range batch {
		pr.Written <- err
	}
}

