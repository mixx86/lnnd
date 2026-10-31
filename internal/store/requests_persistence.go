package store

import (
	"bufio"
	"encoding/binary"

	"fmt"
	"io"
	"lnn/internal/protocol"
	"lnn/internal/queues"
	"lnn/internal/sched"
	"os"
)

func (r *RequestsFile) writeEntry(
	bw *bufio.Writer,
	op opKind,
	fields []byte,
) error {
	entryLength := uint32(entryHeaderLen + len(fields))
	header := encodeEntryHeader(op, entryLength)

	if _, err := bw.Write(header[:]); err != nil {
		return err
	}
	if _, err := bw.Write(fields); err != nil {
		return err
	}

	return nil
}

func (w *RequestsWriter) persistRequests(
	batch []queues.PendingRequest,
	s *sched.Service,
) error {
	reqIDs := make([]uint32, len(batch))
	for i := range batch {
		reqIDs[i] = w.reqFile.nextReqID()
	}
	go func() {
		lastIdx := len(batch) - 1
		var flush bool
		for idx, pr := range batch {
			if idx == lastIdx {
				flush = true
			}
			if err := w.reqCache.Put(reqIDs[idx], pr.Tag, flush); err != nil {
				return
			}
		}
	}()

	f := w.reqFile.file
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	bw := bufio.NewWriter(w.reqFile.file)

	if err := w.reqFile.writeBatch(w.reqFile.file, bw, batch, reqIDs, opRequest); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	wturn := s.WaitWriteTurn(sched.RequestsID)
	defer s.ReleaseWriteTurn(wturn)
	if err := bw.Flush(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	if err := w.reqFile.file.Sync(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	lco, err := w.reqFile.checkpoint(w.reqFile.file, bw)
	if err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	if err := bw.Flush(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}
	if err := w.reqFile.file.Sync(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	if err := w.reqFile.setLco(lco); err != nil {
		notifyWriteResult(batch, err)
		return err
	}
	if err := w.reqFile.file.Sync(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	notifyWriteResult(batch, nil)
	return nil
}

// WARNING: writeBatch does ONLY write in the buffer, make sure to call bw.Flush and f.Sync
// once you're done writing unless you don't actually want to save the content.
func (r *RequestsFile) writeBatch(
	f *os.File,
	bw *bufio.Writer,
	batch []queues.PendingRequest,
	reqIDs []uint32,
	op opKind,
) error {
	for idx, pr := range batch {
		var fields []byte

		switch op {
		case opRequest:
			reqPaths := pr.Req.Encode()

			req := make([]byte, requestStaticFieldsLen+len(reqPaths))

			offset := 0

			binary.BigEndian.PutUint16(
				req[offset:offset+requestTypeLen],
				uint16(pr.Req.Type()),
			)
			offset += requestTypeLen

			copy(req[offset:offset+requestPeerUUIDLen], pr.From.ID[:])
			offset += requestPeerUUIDLen

			binary.BigEndian.PutUint32(
				req[offset:offset+requestIDLen],
				reqIDs[idx],
			)
			offset += requestIDLen

			binary.BigEndian.PutUint64(
				req[offset:offset+requestUnixTSLen],
				pr.EnqueuedAt,
			)
			offset += requestUnixTSLen

			copy(req[offset:], reqPaths)

			fields = req

		case opAccepted, opRejected:
			if res, ok := pr.Req.(protocol.Resolution); ok {
				fieldsBuf := make([]byte, requestIDLen)
				binary.BigEndian.PutUint32(fieldsBuf, res.LCLReqID)
				fields = fieldsBuf
			} else {
				return fmt.Errorf("invalid resolution request")
			}

		default:
			return fmt.Errorf("unknown operation kind: %d", op)
		}

		entryLength := uint32(entryHeaderLen + len(fields))
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

func (r *RequestsFile) checkpoint(
	f *os.File,
	bw *bufio.Writer,
) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}

	lco := uint64(info.Size())

	prevLco, err := r.getLco()
	if err != nil {
		return 0, err
	}

	checkpointEnd := prevLco + checkpointLen

	switch {
	case checkpointEnd == info.Size():
		return 0, fmt.Errorf("cannot put a new checkpoint without new entries")
	case checkpointEnd > info.Size():
		return 0, fmt.Errorf("invalid last checkpoint offset")
	}

	sr := io.NewSectionReader(f, checkpointEnd, info.Size()-checkpointEnd)

	var checksum uint32
	var entryLengthBuf [entryLengthLen]byte
	var entryOffset int64

	for entryOffset < sr.Size() {
		if int64(entryLengthLen) > sr.Size()-entryOffset {
			return 0, fmt.Errorf(
				"truncated entry length at offset %d",
				checkpointEnd+entryOffset,
			)
		}

		if _, err := io.ReadFull(sr, entryLengthBuf[:]); err != nil {
			return 0, fmt.Errorf(
				"reading entry length at offset %d: %w",
				checkpointEnd+entryOffset,
				err,
			)
		}

		entryLength := binary.BigEndian.Uint32(entryLengthBuf[:])

		if entryLength < uint32(entryHeaderLen) {
			return 0, fmt.Errorf(
				"invalid entry length %d at offset %d",
				entryLength,
				checkpointEnd+entryOffset,
			)
		}

		if uint64(entryLength) > uint64(sr.Size()-entryOffset) {
			return 0, fmt.Errorf(
				"truncated entry at offset %d: length=%d, remaining=%d",
				checkpointEnd+entryOffset,
				entryLength,
				sr.Size()-entryOffset,
			)
		}

		entry := make([]byte, entryLength)
		copy(entry[:entryLengthLen], entryLengthBuf[:])

		if _, err := io.ReadFull(sr, entry[entryLengthLen:]); err != nil {
			return 0, fmt.Errorf(
				"reading entry at offset %d: %w",
				checkpointEnd+entryOffset,
				err,
			)
		}

		checksum += calculateChecksum(entry)
		entryOffset += int64(entryLength)
	}

	var fields [checkpointChecksumLen]byte
	binary.BigEndian.PutUint32(fields[:], checksum)

	if err := r.writeEntry(bw, opCheckpoint, fields[:]); err != nil {
		return 0, err
	}

	return lco, nil
}

func notifyWriteResult(batch []queues.PendingRequest, err error) {
	for _, pr := range batch {
		pr.Written <- err
	}
}
