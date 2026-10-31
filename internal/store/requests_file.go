package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"lnn/internal/logger"
	"os"
)

// File header format:
// [magic:4][lco:8][current_req_id:4]
// lco stands for "last checkpoint offset"
// Entry format (one per line):
// Entry header:
//
// [entry_length:4][op:1]
//
// Payload:
//
// Request (op=0):
// [req_type:2][peer_uuid:16][req_id:4][wip: rmt_req_id:4][unix_ts:8]
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

/*
 * `rmt_req_id` should only be used to inform the peer whether a request was accepted or rejected. It shouldn't be
 * used for recovery purposes, as doing so would require inferring state that
 * can't be reliably determined from this field alone.

 * For example, if we send a `RecoverRequest` (without Tag, just the rmt_req_id), we can't know whether
 * the peer's corresponding request was successfully persisted before our own request was interrupted,
 * or whether that request is corrupted or does not exist at all.
 * Similarly, after an unexpected crash, we can't guarantee that `rmt_req_id` itself is consistent
 * or up to date. Using it for recovery would therefore require additional persistent state to track
 * the status of each request, increasing storage overhead while still introducing another piece of
 * state that must itself be recovered.
 * This cannot be solved reliably by simply relying on `rmt_req_id`.

 * We already have a dedicated mechanism for this purpose: the semi-cache based on pairs 'reqId' (local) 'tag'.
 * It allows us to determine which communication was in progress without introducing significant overhead. The cache is written to
 * in parallel through a lightweight, optimized routine that performs the writes.
 */

func OpenRequestsFile(path string) (*RequestsFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		logger.Daemon.Fatalf(
			"error: something went wrong while opening the requests file at: %v: %v",
			path,
			err,
		)
	}

	reqFile := &RequestsFile{
		path: path,
		file: f,
	}

	info, err := reqFile.file.Stat()
	if err != nil {
		if os.IsNotExist(err) {
			reqFile.writeFileHeader()
		} else {
			logger.Daemon.Fatalf("error: %v", err)
		}
	}

	if info.IsDir() {
		logger.Daemon.Fatalf(
			"error: requests shouldn't be a directory at %v",
			path,
		)
	}

	if info.Size() == 0 {
		reqFile.writeFileHeader()
		return reqFile, nil
	}

	reset := func() {
		if err := f.Truncate(0); err != nil {
			logger.Daemon.Fatalf("error truncating requests file: %v", err)
		}
		reqFile.writeFileHeader()
	}

	for {
		err := reqFile.checkFileHeader()
		if err == nil {
			break
		}

		if errors.Is(err, ErrInvalidMagic) {
			reqFile.overrideMagic()
			continue
		}

		if errors.Is(err, ErrInvalidLCO) {
			lco, err := reqFile.findLCO()
			if err == nil {
				if err := reqFile.setLco(lco); err == nil {
					continue
				}
			}
		}

		reset()
		break
	}

	return reqFile, nil
}

func (r *RequestsFile) overrideMagic() error {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], fileHeaderMagic)
	if _, err := r.file.WriteAt(buf[:], 0); err != nil {
		return err
	}
	return r.file.Sync()
}

// writeFileHeader assumes the file is empty; thus, it creates a first
// checkpoint, which is not part of the file header.
func (r *RequestsFile) writeFileHeader() {
	var header [fileHeaderLen]byte
	binary.BigEndian.PutUint32(header[:fileHeaderMagicLen], fileHeaderMagic)
	binary.BigEndian.PutUint64(
		header[fileHeaderMagicLen:fileHeaderMagicLen+fileHeaderLCOLen],
		fileHeaderLen,
	)
	binary.BigEndian.PutUint32(
		header[fileHeaderMagicLen+fileHeaderLCOLen:fileHeaderMagicLen+fileHeaderLCOLen+fileHeaderReqIDLen],
		0,
	)

	var checkpoint [checkpointLen]byte
	binary.BigEndian.PutUint32(checkpoint[:entryLengthLen], checkpointLen)
	checkpoint[entryLengthLen] = uint8(opCheckpoint)
	binary.BigEndian.PutUint32(
		checkpoint[entryHeaderLen:entryHeaderLen+checkpointChecksumLen],
		uint32(0),
	)

	if _, err := r.file.Write(header[:]); err != nil {
		logger.Daemon.Fatalf("error writing the file header: %v", err)
	}
	if _, err := r.file.Write(checkpoint[:]); err != nil {
		logger.Daemon.Fatalf("error writing the initial checkpoint: %v", err)
	}

	if err := r.file.Sync(); err != nil {
		logger.Daemon.Fatalf("error writing the file header: %v", err)
	}
}

func (r *RequestsFile) setLco(lco uint64) error {
	var buf [fileHeaderLCOLen]byte

	binary.BigEndian.PutUint64(buf[:], lco)

	if _, err := r.file.WriteAt(
		buf[:],
		fileHeaderMagicLen,
	); err != nil {
		return fmt.Errorf("writing lco: %w", err)
	}

	return nil
}

func (r *RequestsFile) getLco() (int64, error) {
	var b [fileHeaderLCOLen]byte

	if _, err := r.file.ReadAt(b[:], fileHeaderMagicLen); err != nil {
		return 0, err
	}

	return int64(binary.BigEndian.Uint64(b[:])), nil
}

func encodeEntryHeader(op opKind, entryLength uint32) [entryHeaderLen]byte {
	var data [entryHeaderLen]byte

	binary.BigEndian.PutUint32(
		data[:entryLengthLen],
		entryLength,
	)
	data[entryLengthLen] = uint8(op)

	return data
}

func calculateChecksum(data []byte) uint32 {
	var checksum uint32
	for _, b := range data {
		checksum += uint32(b)
	}
	return checksum
}

func (r *RequestsFile) resetToLastValidCheckpoint() error {
	checkpoint, err := r.findLCO()
	if err != nil {
		return err
	}

	checkpointEnd := int64(checkpoint) + int64(checkpointLen)

	if err := r.file.Truncate(checkpointEnd); err != nil {
		return err
	}

	if err := r.setLco(checkpoint); err != nil {
		return err
	}

	return r.file.Sync()
}

func (r *RequestsFile) removeTail() error {
	info, err := r.file.Stat()
	if err != nil {
		return err
	}

	lco, err := r.getLco()
	if err != nil {
		return err
	}

	checkpointEnd := int64(lco) + int64(checkpointLen)

	_, entryLength, op, err := r.readEntry(lco, info.Size())
	if err != nil || op != opCheckpoint ||
		entryLength != uint32(checkpointLen) {
		return r.resetToLastValidCheckpoint()
	}

	if checkpointEnd > info.Size() {
		return r.resetToLastValidCheckpoint()
	}

	// The last checkpoint is valid, so everything after it is tail.
	if checkpointEnd < info.Size() {
		if err := r.file.Truncate(checkpointEnd); err != nil {
			return fmt.Errorf(
				"truncating requests tail at %d: %w",
				checkpointEnd,
				err,
			)
		}

		return r.file.Sync()
	}

	// The last entry is the checkpoint itself. If we got here because
	// checkEntries found corruption, the checkpoint checksum is invalid.
	previousCheckpoint, err := r.findPreviousCheckpoint(int64(lco))
	if err != nil {
		// The initial checkpoint is the only checkpoint and there is
		// therefore nothing before it to recover.
		if errors.Is(err, ErrNoPreviousCheckpoint) {
			if err := r.file.Truncate(fileHeaderLen + checkpointLen); err != nil {
				return err
			}

			return r.file.Sync()
		}

		return err
	}

	previousCheckpointEnd :=
		previousCheckpoint + int64(checkpointLen)

	if err := r.file.Truncate(previousCheckpointEnd); err != nil {
		return fmt.Errorf(
			"truncating corrupted checkpoint at %d: %w",
			lco,
			err,
		)
	}

	if err := r.setLco(uint64(previousCheckpoint)); err != nil {
		return err
	}

	return r.file.Sync()
}
