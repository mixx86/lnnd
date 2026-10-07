package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"lnn/internal/logger"
	"os"
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

