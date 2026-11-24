package store

import (
	"encoding/binary"
	"errors"
	"fmt"
	"lnn/internal/logger"

	"github.com/google/uuid"
)

func (r *RequestsFile) checkFileHeader() error {
	header := make([]byte, fileHeaderLen)
	if _, err := r.file.ReadAt(header, 0); err != nil {
		return err
	}

	magic := binary.BigEndian.Uint32(header[:fileHeaderMagicLen])
	if magic != fileHeaderMagic {
		return ErrInvalidMagic
	}

	lco := binary.BigEndian.Uint64(
		header[fileHeaderMagicLen : fileHeaderMagicLen+fileHeaderLCOLen],
	)

	info, err := r.file.Stat()
	if err != nil {
		logger.Daemon.Fatalf(
			"error: couldn't stat requests file at %v: %v",
			r.path,
			err,
		)
	}

	if lco < fileHeaderLen || lco >= uint64(info.Size()) {
		return ErrInvalidLCO
	}

	return nil
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
	checkpointEnd := int64(lco) + int64(checkpointLen)

	if int64(lco) < int64(fileHeaderLen) || checkpointEnd > fileSize {
		return fmt.Errorf("invalid last checkpoint offset: %d", lco), false
	}

	_, entryLength, op, err := r.readEntry(lco, fileSize)
	if err != nil {
		return err, false
	}

	if op != opCheckpoint || entryLength != uint32(checkpointLen) {
		return fmt.Errorf("invalid checkpoint at lco %d", lco), false
	}

	// There are entries after the last checkpoint. Validate only the
	// uncheckpointed tail using entry_length.
	if checkpointEnd < fileSize {
		return r.checkTailEntries(checkpointEnd, fileSize)
	}

	// The last entry is the last checkpoint. Find the previous
	// checkpoint so we can validate only the region it closes.
	previousCheckpoint, err := r.findPreviousCheckpoint(lco)
	if errors.Is(err, ErrNoPreviousCheckpoint) &&
		lco == int64(fileHeaderLen) {
		// Only the initial checkpoint exists: a fresh file.
		return nil, false
	}
	if err != nil {
		return err, false
	}

	offset := previousCheckpoint + int64(checkpointLen)
	var checksum uint32
	var recoverable bool

	for offset < int64(lco) {
		entry, entryLength, op, err := r.readEntry(offset, lco)
		if err != nil {
			return err, recoverable
		}

		if op == opRequest &&
			entryLength >= uint32(requestRecoveryMinLen) &&
			sentByPeer(entry) {
			r.recovery = append(r.recovery, RecoveryEntry{
				Offset: offset,
				PeerUUID: uuid.UUID(
					entry[requestPeerUUIDOffset : requestPeerUUIDOffset+
						requestPeerUUIDLen],
				),
				ReqID: binary.BigEndian.Uint32(
					entry[requestIDOffset : requestIDOffset+requestIDLen],
				),
			})
			recoverable = true
		}

		checksum += calculateChecksum(entry)
		offset += int64(entryLength)
	}

	if offset != int64(lco) {
		return fmt.Errorf(
			"entries do not align with last checkpoint at offset %d",
			lco,
		), recoverable
	}

	var checksumBuf [checkpointChecksumLen]byte

	if _, err := r.file.ReadAt(
		checksumBuf[:],
		int64(lco)+int64(entryHeaderLen),
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
			minRequestLen := int64(requestRecoveryMinLen)

			if remaining >= minRequestLen {
				var opBuf [opKindLen]byte

				if _, readErr := r.file.ReadAt(
					opBuf[:],
					offset+int64(entryLengthLen),
				); readErr != nil {
					return err, false
				}

				if opKind(opBuf[0]) == opRequest {
					added, rerr := r.addTailRecovery(offset)
					if rerr != nil {
						return err, false
					}
					return err, added
				}
			}

			return err, false
		}

		// A checkpoint after lco would mean the file header's lco is stale.
		// It should not happen if lco is maintained correctly.
		if op == opCheckpoint {
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

func (r *RequestsFile) findLCO() (uint64, error) {
	info, err := r.file.Stat()
	if err != nil {
		return 0, err
	}

	lastCheckpoint := int64(-1)
	offset := int64(fileHeaderLen)

	for offset < info.Size() {
		_, entryLength, op, err := r.readEntry(offset, info.Size())
		if err != nil {
			break
		}

		if op == opCheckpoint {
			if entryLength != uint32(checkpointLen) {
				break
			}
			lastCheckpoint = offset
		}

		offset += int64(entryLength)
	}

	if lastCheckpoint == -1 {
		return 0, ErrNoPreviousCheckpoint
	}

	return uint64(lastCheckpoint), nil
}

func (r *RequestsFile) findPreviousCheckpoint(
	lco int64,
) (int64, error) {
	offset := int64(fileHeaderLen)
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

		if op == opCheckpoint {
			if entryLength != uint32(checkpointLen) {
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
) ([]byte, uint32, opKind, error) {
	if offset < 0 || offset >= limit {
		return nil, 0, 0, fmt.Errorf(
			"invalid entry offset %d",
			offset,
		)
	}

	if limit-offset < int64(entryLengthLen) {
		return nil, 0, 0, fmt.Errorf(
			"%w at offset %d",
			ErrTruncatedEntry,
			offset,
		)
	}

	var lengthBuf [entryLengthLen]byte
	if _, err := r.file.ReadAt(lengthBuf[:], offset); err != nil {
		return nil, 0, 0, fmt.Errorf(
			"reading entry length at offset %d: %w",
			offset,
			err,
		)
	}

	entryLength := binary.BigEndian.Uint32(lengthBuf[:])

	if entryLength < uint32(entryHeaderLen) {
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

	op := opKind(entry[entryLengthLen])

	switch op {
	case opRequest, opAccepted, opRejected, opCheckpoint:
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

// A request of ours has rmt_req_id 0: the peer owes nothing for it, so it
// is never asked to resend it.
func sentByPeer(entry []byte) bool {
	return binary.BigEndian.Uint32(
		entry[requestRMTReqIDOffset:requestRMTReqIDOffset+requestRMTReqIDLen],
	) != 0
}

func (r *RequestsFile) addTailRecovery(offset int64) (bool, error) {
	var buf [requestRecoveryMinLen]byte
	if _, err := r.file.ReadAt(buf[:], offset); err != nil {
		return false, err
	}
	if !sentByPeer(buf[:]) {
		return false, nil
	}
	r.recovery = append(r.recovery, RecoveryEntry{
		Offset: offset,
		PeerUUID: uuid.UUID(
			buf[requestPeerUUIDOffset : requestPeerUUIDOffset+
				requestPeerUUIDLen],
		),
		ReqID: binary.BigEndian.Uint32(
			buf[requestIDOffset : requestIDOffset+requestIDLen],
		),
	})
	return true, nil
}
