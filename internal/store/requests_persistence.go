package store

import (
	"bufio"
	"encoding/binary"

	"fmt"
	"io"
	"lnn/internal/logger"
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

	/* The cache does not take a write turn (for now?):
	 * a few bytes per request are not worth scheduling. Still
	 * being reconsidered, which is why RequestsCacheID keeps its
	 * WriterConfig in sched.
	 */
	go func() {
		lastIdx := -1
		for idx, pr := range batch {
			if !pr.Sent {
				lastIdx = idx
			}
		}
		for idx, pr := range batch {
			if pr.Sent {
				continue
			}
			flush := idx == lastIdx
			if err := w.reqCache.Put(reqIDs[idx], pr.Tag, flush); err != nil {
				return
			}
		}
	}()

	f := w.reqFile.file
	start, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	prevLco, err := w.reqFile.getLco()
	if err != nil {
		return err
	}
	w.batchBuf.Reset()
	bw := bufio.NewWriter(w.reqFile.file)

	err = w.reqFile.writeBatch(
		w.reqFile.file,
		&w.batchBuf,
		batch,
		reqIDs,
		opRequest,
	)
	if err != nil {
		notifyWriteResult(batch, reqIDs, err)
		return err
	}

	written := int64(w.batchBuf.Len()) + checkpointLen + fileHeaderLCOLen

	wturn := s.WaitWriteTurn(sched.RequestsID)
	_, err = w.batchBuf.WriteTo(bw)
	if err == nil {
		err = w.commitBatch(bw)
	}
	if err != nil {
		// A partial write (e.g. disk full) leaves a cut entry at the tail.
		// Later batches would land after it and every checkpoint would
		// fail parsing it until a restart repaired the file.
		if rerr := w.rollbackBatch(start, prevLco); rerr != nil {
			logger.Daemon.Printf("rolling back failed batch: %v", rerr)
		}
		s.ReleaseWriteTurn(wturn, 0)
		notifyWriteResult(batch, reqIDs, err)
		return err
	}
	s.ReleaseWriteTurn(wturn, written)

	notifyWriteResult(batch, reqIDs, nil)
	return nil
}

// rollbackBatch restores the file to how it was before the batch, lco
// included: commitBatch may have already pointed lco at a checkpoint
// past end. Going back to the last complete checkpoint instead would
// keep a batch that its writers were told failed.
func (w *RequestsWriter) rollbackBatch(end, lco int64) error {
	f := w.reqFile.file
	if err := f.Truncate(end); err != nil {
		return err
	}
	if err := w.reqFile.setLco(uint64(lco)); err != nil {
		return err
	}
	return f.Sync()
}

func (w *RequestsWriter) commitBatch(bw *bufio.Writer) error {
	f := w.reqFile.file

	if err := bw.Flush(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	lco, err := w.reqFile.checkpoint(f, bw)
	if err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	if err := w.reqFile.setLco(lco); err != nil {
		return err
	}
	return f.Sync()
}

// WARNING: writeBatch does ONLY encode into buf, write it to the file and
// call f.Sync yourself or the content is never saved.
func (r *RequestsFile) writeBatch(
	f *os.File,
	buf io.Writer,
	batch []queues.PendingRequest,
	reqIDs []uint32,
	op opKind,
) error {
	for idx, pr := range batch {
		var fields []byte
		entryOp := op
		if op == opRequest && pr.Sent {
			entryOp = opSentRequest
		}

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

			copy(req[offset:offset+requestPeerUUIDLen], pr.Peer.ID[:])
			offset += requestPeerUUIDLen

			binary.BigEndian.PutUint32(
				req[offset:offset+requestIDLen],
				reqIDs[idx],
			)
			offset += requestIDLen

			binary.BigEndian.PutUint32(
				req[offset:offset+requestRMTReqIDLen],
				pr.RMTReqID,
			)
			offset += requestRMTReqIDLen

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
		header := encodeEntryHeader(entryOp, entryLength)

		req := make([]byte, 0, entryLength)
		req = append(req, header[:]...)
		req = append(req, fields...)
		if _, err := buf.Write(req); err != nil {
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

func notifyWriteResult(
	batch []queues.PendingRequest,
	reqIDs []uint32,
	err error,
) {
	for idx, pr := range batch {
		res := queues.WriteResult{Err: err}
		if err == nil {
			res.ReqID = reqIDs[idx]
		}
		pr.Written <- res
	}
}
