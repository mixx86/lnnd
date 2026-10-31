package store

import (
	"encoding/binary"
	"fmt"
	"lnn/internal/network"
	"lnn/internal/protocol"
	"os"
)

func (w *RequestsWriter) recoverEntries() error {
	for _, entry := range w.reqFile.recovery {
		tag, ok := w.reqCache.GetTag(entry.ReqID)
		if !ok {
			continue
		}

		if err := w.recoverRequest(
			entry.PeerUUID,
			entry.ReqID,
			tag,
		); err != nil {
			return fmt.Errorf(
				"recovering request %d at offset %d: %w",
				entry.ReqID,
				entry.Offset,
				err,
			)
		}
	}

	return nil
}

func (w *RequestsWriter) recoverRequest(
	peerUUID [16]byte,
	reqID uint32,
	tag uint16,
) error {
	s, ok := w.sm.Get(peerUUID)
	if !ok {
		return fmt.Errorf("no peer with uuid %v was found", peerUUID)
	}

	err := network.SendFrame(s, protocol.Frame{
		Body: protocol.RecoverRequest{},
	}.WithTag(tag))
	if err != nil {
		return err
	}

	return nil
}

func (w *RequestsWriter) writeRecoveredRequest(
	f *os.File,
	request []byte,
) error {
	if len(request) < ENTRY_HEADER_LEN {
		return fmt.Errorf("invalid recovered request length %d", len(request))
	}

	entryLength := binary.BigEndian.Uint32(
		request[:ENTRY_LENGTH_LEN],
	)

	if int(entryLength) != len(request) {
		return fmt.Errorf(
			"recovered request length mismatch: header=%d actual=%d",
			entryLength,
			len(request),
		)
	}

	if OP_KIND(request[ENTRY_LENGTH_LEN]) != OP_REQUEST {
		return fmt.Errorf(
			"recovered entry is not a request: op=%d",
			request[ENTRY_LENGTH_LEN],
		)
	}

	_, err := f.Write(request)
	return err
}
