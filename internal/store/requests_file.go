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


func OpenRequestsFile(path string) (*RequestsFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		logger.Daemon.Fatalf("error: something went wrong while opening the requests file at: %v: %v", path, err)
	}

	reqFile := &RequestsFile{
		path:  path,
		file: f,
	}

	info, err := reqFile.file.Stat()
	if err != nil {
		if os.IsNotExist(err) {
			reqFile.writeRequestsFileHeader(f)
		} else {
			logger.Daemon.Fatalf("error: %v", err)
		}
	}

	if info.IsDir() {
		logger.Daemon.Fatalf("error: requests shouldn't be a directory at %v", path)
	}

	if info.Size() == 0 {
		reqFile.writeRequestsFileHeader(f)
	}

	if !reqFile.checkRequestsFileHeader(f) {
		if err := f.Truncate(0); err != nil {
			logger.Daemon.Fatalf("error truncating requests file: %v", err)
		}
		reqFile.writeRequestsFileHeader(f)
	}

	return reqFile, nil
}

// writeRequestsFileHeader assumes the file is empty; thus, it creates a first
// checkpoint, which is not part of the file header.
func (r *RequestsFile) writeRequestsFileHeader(f *os.File) {
	var header [F_HEADER_TOTAL_LEN]byte
	binary.BigEndian.PutUint32(header[:F_HEADER_MAGIC_LEN], F_HEADER_MAGIC)
	binary.BigEndian.PutUint64(header[F_HEADER_MAGIC_LEN:F_HEADER_MAGIC_LEN+F_HEADER_LCO_LEN], F_HEADER_TOTAL_LEN)
	binary.BigEndian.PutUint32(header[F_HEADER_MAGIC_LEN+F_HEADER_LCO_LEN:F_HEADER_MAGIC_LEN+F_HEADER_LCO_LEN+F_HEADER_REQ_ID_LEN], 0)

	var checkpoint [CHECKPOINT_LEN]byte
	binary.BigEndian.PutUint32(checkpoint[:ENTRY_LENGTH_LEN], CHECKPOINT_LEN)
	checkpoint[ENTRY_LENGTH_LEN] = uint8(OP_CHECKPOINT)
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

func (r *RequestsFile) setLco(lco uint64) error {
	var buf [F_HEADER_LCO_LEN]byte

	binary.BigEndian.PutUint64(buf[:], lco)

	if _, err := r.file.WriteAt(
		buf[:],
		F_HEADER_MAGIC_LEN,
	); err != nil {
		return fmt.Errorf("writing lco: %w", err)
	}

	return nil
}

func (r *RequestsFile) getLco() (int64, error) {
	var b [F_HEADER_LCO_LEN]byte

	if _, err := r.file.ReadAt(b[:], F_HEADER_MAGIC_LEN); err != nil {
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

func (r *RequestsFile) removeTail() error {
	info, err := r.file.Stat()
	if err != nil {
		return err
	}
		
	lco, err := r.getLco()
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
			if err := r.file.Truncate(F_HEADER_TOTAL_LEN + CHECKPOINT_LEN); err != nil {
				return err
			}

			return r.file.Sync()
		}

		return err
	}

	previousCheckpointEnd :=
		previousCheckpoint + int64(CHECKPOINT_LEN)

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

