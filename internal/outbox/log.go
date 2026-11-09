package outbox

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"sync"

	"github.com/google/uuid"

	"lnn/internal/fsutil"
	"lnn/internal/logger"
	"lnn/internal/protocol"
)

// Record format:
//
//	[record_len:4][crc32:4][op:1][peer_uuid:16][tag:4][body]
//
// crc32 covers everything after itself. Sent records carry
// [msg_type:1][payload] as body; removed and tag records carry none.
const (
	recordLenLen  = 4
	recordCRCLen  = 4
	recordOpLen   = 1
	recordPeerLen = 16
	recordTagLen  = 4

	recordCRCOffset  = recordLenLen
	recordOpOffset   = recordCRCOffset + recordCRCLen
	recordPeerOffset = recordOpOffset + recordOpLen
	recordTagOffset  = recordPeerOffset + recordPeerLen
	recordBodyOffset = recordTagOffset + recordTagLen

	recordMsgTypeLen = 1
)

type recordOp uint8

const (
	opSent recordOp = iota
	opRemoved
	// opTag only appears in compacted files: once the sent records of a
	// peer are gone, it is the only place its last tag survives.
	opTag
)

type logFile struct {
	path string
	file *os.File
	size int64
}

func Open(path string) (*Outbox, error) {
	o := &Outbox{
		peers:   make(map[uuid.UUID]*window),
		wake:    make(chan struct{}, 1),
		stopped: make(chan struct{}),
	}
	o.drained = sync.NewCond(&o.mu)

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	o.replay(data)

	for _, w := range o.peers {
		w.lastTag += restartTagSkip
	}

	o.log = &logFile{path: path}
	if err := o.compact(); err != nil {
		return nil, err
	}

	go o.run()
	return o, nil
}

// replay stops at the first record that is short or fails its checksum:
// the writer only appends, so anything after it is a torn write.
func (o *Outbox) replay(data []byte) {
	for len(data) >= recordBodyOffset {
		n := binary.BigEndian.Uint32(data[:recordLenLen])
		if n < recordBodyOffset || int(n) > len(data) {
			return
		}
		record := data[:n]
		crc := binary.BigEndian.Uint32(record[recordCRCOffset:recordOpOffset])
		if crc32.ChecksumIEEE(record[recordOpOffset:]) != crc {
			return
		}
		if err := o.apply(record); err != nil {
			logger.Daemon.Printf("outbox: skipping record: %v", err)
		}
		data = data[n:]
	}
}

func (o *Outbox) apply(record []byte) error {
	peer := uuid.UUID(record[recordPeerOffset:recordTagOffset])
	tag := binary.BigEndian.Uint32(record[recordTagOffset:recordBodyOffset])
	body := record[recordBodyOffset:]
	w := o.window(peer)

	switch recordOp(record[recordOpOffset]) {
	case opSent:
		if len(body) < recordMsgTypeLen {
			return fmt.Errorf("sent record without message type")
		}
		t := protocol.MsgType(body[0])
		payload, err := protocol.ParsePayload(t, body[recordMsgTypeLen:])
		if err != nil {
			return err
		}
		if _, exists := w.frames[tag]; !exists {
			w.unackedTags = append(w.unackedTags, tag)
		}
		w.frames[tag] = protocol.Frame{Body: payload}.WithTag(tag)
		w.lastTag = tag
	case opRemoved:
		delete(w.frames, tag)
		for i, t := range w.unackedTags {
			if t == tag {
				w.unackedTags = append(
					w.unackedTags[:i],
					w.unackedTags[i+1:]...,
				)
				break
			}
		}
	case opTag:
		w.lastTag = tag
	default:
		return fmt.Errorf("unknown op %d", record[recordOpOffset])
	}
	return nil
}

func encodeRecord(
	op recordOp,
	peer uuid.UUID,
	tag uint32,
	body []byte,
) []byte {
	record := make([]byte, recordBodyOffset+len(body))
	binary.BigEndian.PutUint32(record[:recordLenLen], uint32(len(record)))
	record[recordOpOffset] = byte(op)
	copy(record[recordPeerOffset:recordTagOffset], peer[:])
	binary.BigEndian.PutUint32(record[recordTagOffset:recordBodyOffset], tag)
	copy(record[recordBodyOffset:], body)
	binary.BigEndian.PutUint32(
		record[recordCRCOffset:recordOpOffset],
		crc32.ChecksumIEEE(record[recordOpOffset:]),
	)
	return record
}

func encodeSent(peer uuid.UUID, frame protocol.Frame) []byte {
	payload := frame.Body.Encode()
	body := make([]byte, recordMsgTypeLen+len(payload))
	body[0] = byte(frame.Body.Type())
	copy(body[recordMsgTypeLen:], payload)
	return encodeRecord(opSent, peer, frame.Header.Tag, body)
}

func encodeRemoved(peer uuid.UUID, tag uint32) []byte {
	return encodeRecord(opRemoved, peer, tag, nil)
}

func (o *Outbox) run() {
	defer close(o.stopped)
	for range o.wake {
		o.mu.Lock()
		batch := o.pending
		o.pending = nil
		o.inflight = len(batch)
		closing := o.closing
		o.mu.Unlock()

		if err := o.log.append(batch); err != nil {
			logger.Daemon.Printf("outbox: writing records: %v", err)
		}

		o.mu.Lock()
		o.inflight = 0
		if o.log.size > compactAfterBytes {
			if err := o.compact(); err != nil {
				logger.Daemon.Printf("outbox: compacting: %v", err)
			}
		}
		o.drained.Broadcast()
		o.mu.Unlock()

		if closing {
			return
		}
	}
}

func (l *logFile) append(records [][]byte) error {
	if len(records) == 0 {
		return nil
	}
	var buf []byte
	for _, r := range records {
		buf = append(buf, r...)
	}
	n, err := l.file.Write(buf)
	l.size += int64(n)
	if err != nil {
		return err
	}
	return l.file.Sync()
}

// compact rewrites the log from memory and must be called with o.mu held.
// Records still pending are already reflected in memory, so they are
// dropped instead of being appended to the new file.
func (o *Outbox) compact() error {
	var buf []byte
	for peer, w := range o.peers {
		for _, tag := range w.unackedTags {
			buf = append(buf, encodeSent(peer, w.frames[tag])...)
		}
		// Must be the peer's last record: otherwise, on replay after a restart,
		// lastTag ends up with the tag of a sent record, which may be smaller.
		buf = append(buf, encodeRecord(opTag, peer, w.lastTag, nil)...)
	}

	if err := fsutil.WriteFileAtomic(o.log.path, buf); err != nil {
		return err
	}

	if o.log.file != nil {
		_ = o.log.file.Close()
	}
	f, err := os.OpenFile(o.log.path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	o.log.file = f
	o.log.size = int64(len(buf))
	o.pending = nil
	return nil
}

// Close flushes every pending record before returning, so a clean
// shutdown never relies on restartTagSkip.
func (o *Outbox) Close() error {
	o.mu.Lock()
	o.closing = true
	o.drained.Broadcast()
	o.mu.Unlock()

	select {
	case o.wake <- struct{}{}:
	default:
	}
	<-o.stopped
	return o.log.file.Close()
}
