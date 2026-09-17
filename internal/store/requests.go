package store

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"lnn/internal/logger"
	"lnn/internal/queues"
	"lnn/internal/sched"
	"os"
	"path/filepath"
	"time"
)

// Cosas a añadir:
// HEADER al inicio del archivo que indique offset de archivo hasta checkpoint
// length para cada campo
// checksum al lado del checkpoint
// checkpoint after drenar batch
// al arrancar validar checksum si no hay entrys delante
// separar entry header de payload header

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

const (
	maxBatch        = 64
	checkpointAfter = 512
)

type OP_KIND uint8

const (
	OP_REQUEST OP_KIND = iota
	OP_ACCEPTED
	OP_REJECTED
	OP_CHECKPOINT
)

// con app podemos sacar sched
func NewRequestsWriter(dir string) *RequestsWriter {
	return &RequestsWriter{
		canPath: filepath.Join(dir, "requests"),
	}
}

func (w *RequestsWriter) Run(s *sched.Service) {
	for {
		req, err := s.NextRequest()
		if err != nil {
			logger.Daemon.Fatalf("error: %v", err)
		}
		batch := []queues.PendingRequest{req}

		for len(batch) < maxBatch {
			req, ok, err := s.TryNextRequest()
			if err != nil {
				logger.Daemon.Fatalf("error: %v", err)
			}

			if !ok {
				break
			}
			batch = append(batch, req)
		}

		w.persistRequests(batch, s)
	}
}

func encodeEntryHeader(op OP_KIND, pr queues.PendingRequest) [31]byte {
	var data [31]byte

	data[0] = uint8(op)
	if op == OP_REQUEST {
		binary.BigEndian.PutUint16(data[1:3], uint16(pr.Req.Type()))

		copy(data[3:19], pr.From.ID[:])

		binary.BigEndian.PutUint32(data[19:23], pr.ID)
		binary.BigEndian.PutUint64(
			data[23:31],
			uint64(time.Now().Unix()),
		)
	} else if op == OP_ACCEPTED || op == OP_REJECTED {
		binary.BigEndian.PutUint32(data[1:5], pr.ID)
	}

	return data
}

func (w *RequestsWriter) writeBatch(bw *bufio.Writer, batch []queues.PendingRequest, op OP_KIND) error {
	for _, pr := range batch {
		header := encodeEntryHeader(op, pr)
		var fields []byte

		switch op {
		case OP_REQUEST:
			fields = pr.Req.Encode()

		case OP_CHECKPOINT, OP_ACCEPTED, OP_REJECTED:
			fields = nil

		default:
			return fmt.Errorf("unknown operation kind: %d", op)
		}

		req := make([]byte, 0, len(header)+len(fields))
		req = append(req, header[:]...)
		req = append(req, fields...)
		if _, err := bw.Write(req); err != nil {
			return err
		}
	}

	return nil
}

func notifyWriteResult(batch []queues.PendingRequest, err error) {
	for _, pr := range batch {
		pr.Written <- err
	}
}

func (w *RequestsWriter) persistRequests(batch []queues.PendingRequest, s *sched.Service) error {
	f, err := os.OpenFile(w.canPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)

	if err := w.writeBatch(bw, batch, OP_REQUEST); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	for s.IsBackPressured(sched.BPRequestsID) {
		if len(batch) >= maxBatch {
			s.WaitForPressureRelease(sched.BPRequestsID)
			break
		}

		req, err := s.NextRequest()
		if err != nil {
			notifyWriteResult(batch, err)
			return err
		}

		batch = append(batch, req)

		if err := w.writeBatch(bw, []queues.PendingRequest{req}, OP_REQUEST); err != nil {
			notifyWriteResult(batch, err)
			return err
		}
	}

	if err = bw.Flush(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}
	if err = f.Sync(); err != nil {
		notifyWriteResult(batch, err)
		return err
	}

	notifyWriteResult(batch, nil)
	return nil
}

func (w *RequestsWriter) checkpoint(f *os.File, bw *bufio.Writer) {
	w.writeBatch(bw, []queues.PendingRequest{}, OP_CHECKPOINT)
}
