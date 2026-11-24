package store

import (
	"encoding/binary"
	"fmt"
	"lnn/internal/recovery"
	"os"
)

// recordRecoveries must run before the damaged region is truncated: once
// it is gone, the pending list is the only trace of what peers still owe.
func (w *RequestsWriter) recordRecoveries() error {
	var entries []recovery.Entry
	for _, e := range w.reqFile.recovery {
		tag, ok := w.reqCache.GetTag(e.LCLReqID)
		if !ok {
			continue
		}
		entries = append(entries, recovery.Entry{
			Peer:     e.PeerUUID,
			LCLReqID: e.LCLReqID,
			Tag:      tag,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return w.pending.Add(entries...)
}

func (w *RequestsWriter) writeRecoveredRequest(
	f *os.File,
	request []byte,
) error {
	if len(request) < entryHeaderLen {
		return fmt.Errorf("invalid recovered request length %d", len(request))
	}

	entryLength := binary.BigEndian.Uint32(
		request[:entryLengthLen],
	)

	if int(entryLength) != len(request) {
		return fmt.Errorf(
			"recovered request length mismatch: header=%d actual=%d",
			entryLength,
			len(request),
		)
	}

	if opKind(request[entryLengthLen]) != opRequest {
		return fmt.Errorf(
			"recovered entry is not a request: op=%d",
			request[entryLengthLen],
		)
	}

	_, err := f.Write(request)
	return err
}
