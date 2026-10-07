package store

import (
	"encoding/binary"
	"fmt"
	"lnn/internal/logger"
	"os"

	"github.com/google/uuid"
)

func (r *RequestsFile) checkRequestsFileHeader(f *os.File) bool {
	header := make([]byte, F_HEADER_TOTAL_LEN)
	if _, err := f.ReadAt(header, 0); err != nil {
		return false
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
			r.path,
			err,
		)
	}

	if lco < F_HEADER_TOTAL_LEN || lco >= uint64(info.Size()) {
		return false
	}

	return true
}

func (r *RequestsFile) checkEntries() (error, bool) {
	info, err := r.file.Stat()
	if err != nil {
		return err, false
	}
	lco, err := r.getLco()
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
		return r.checkTailEntries(checkpointEnd, fileSize)
	}

	// The last entry is the last checkpoint. Find the previous
	// checkpoint so we can validate only the region it closes.
	previousCheckpoint, err := r.findPreviousCheckpoint(lco)
	if err != nil {
		return err, false
	}

	offset := previousCheckpoint + int64(CHECKPOINT_LEN)
	var checksum uint32
	var recoverable bool

	for offset < int64(lco) {
		entry, entryLength, op, err := r.readEntry(offset, lco)
		if err != nil {
			return err, recoverable
		}

		if op == OP_REQUEST &&
			entryLength >= uint32(REQUEST_RECOVERY_MIN_LEN) {
			r.recovery = append(r.recovery, RecoveryEntry{
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

	if _, err := r.file.ReadAt(
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

func (r *RequestsFile) checkTailEntries(
	start int64,
	fileSize int64,
) (error, bool) {
	offset := start

	for offset < fileSize {
		entry, entryLength, op, err := r.readEntry(offset, fileSize)
		if err != nil {
			// The entry is truncated/corrupt. Check whether enough of the
			// request survived to recover peer_uuid + req_id.
			remaining := fileSize - offset
			minRequestLen := int64(REQUEST_RECOVERY_MIN_LEN)

			if remaining >= minRequestLen {
				var opBuf [OP_KIND_LEN]byte

				if _, readErr := r.file.ReadAt(
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

func (r *RequestsFile) findPreviousCheckpoint(
	lco int64,
) (int64, error) {
	offset := int64(F_HEADER_TOTAL_LEN)
	previousCheckpoint := int64(-1)

	for offset < lco {
		_, entryLength, op, err := r.readEntry(offset, lco)
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

func (r *RequestsFile) readEntry(
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
	if _, err := r.file.ReadAt(lengthBuf[:], offset); err != nil {
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
	if _, err := r.file.ReadAt(entry, offset); err != nil {
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

