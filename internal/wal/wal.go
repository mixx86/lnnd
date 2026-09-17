// Package wal implements a write-ahead log for inbound requests.
//
// Two files live under the data directory:
//   - requests     — canonical state (compacted, rewritten atomically on checkpoint)
//   - requests.wal — append-only log; replayed after the last checkpoint marker on startup
//
// Entry format (one per line):
//
//	R|<unix_ts>|<req_type>|<peer_uuid>|<req_id>|<field0>|<field1>|...
//	A|<unix_ts>|<req_id>
//	X|<unix_ts>|<req_id>
//	K|<unix_ts>
//
// R=request, A=accepted, X=rejected, K=checkpoint.
// <req_type> is the protocol message type byte (e.g. 6=SymLinkRequest).
// Fields after <req_id> are type-specific; '|' inside field values is percent-encoded as %7C.
// TODO: Add CRC, do this to binary instead of text, use just one file (`requests`) for everything instead of two,
// and this is not a wal, we'll use a CRC for corrupted data, since we don't have big data amount, and this would go on store/requests.go,
// and the req queue manager would go on the package queue, as requests.go
// we have to also add constancy for the sent requests that have either not reply or en error reply, so we can resend those requests
package wal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"lnn/internal/iocore"
	"lnn/internal/protocol"
)

const (
	maxBatch        = 64
	checkpointAfter = 512
)

// Writer drains PendingRequests from the iocore request channel and persists them
// to the WAL, backing off when io_uring pressure is high. Run it in a dedicated goroutine.
type Writer struct {
	ioc     *iocore.Manager
	walPath string
	canPath string

	mu       sync.Mutex
	walLines int
}

// NewWriter creates a Writer that stores files under dir.
func NewWriter(dir string, ioc *iocore.Manager) *Writer {
	return &Writer{
		ioc:     ioc,
		walPath: filepath.Join(dir, "requests.wal"),
		canPath: filepath.Join(dir, "requests"),
	}
}

// Run starts the write loop. It blocks until the request channel is closed.
func (w *Writer) Run() {
	ch := w.ioc.ReqCh()
	for req := range ch {
		batch := []iocore.PendingRequest{req}
	drain:
		for len(batch) < maxBatch {
			select {
			case r := <-ch:
				batch = append(batch, r)
			default:
				break drain
			}
		}

		// Back off while io_uring threads are active — they have priority over disk I/O.
	backoff:
		for w.ioc.Pressure() > 0 {
			select {
			case r := <-ch:
				if len(batch) < maxBatch {
					batch = append(batch, r)
				}
			case <-w.ioc.PressureDone():
				break backoff
			}
		}

		if err := w.writeBatch(batch); err != nil {
			fmt.Fprintf(os.Stderr, "wal: write error: %v\n", err)
		}
	}
}

func (w *Writer) writeBatch(batch []iocore.PendingRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.OpenFile(w.walPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	ts := time.Now().Unix()
	for _, pr := range batch {
		fields, err := encodeFields(pr.Req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wal: skipping request %d: %v\n", pr.ID, err)
			continue
		}
		fmt.Fprintf(bw, "R|%d|%d|%s|%d|%s\n", ts, pr.Req.Type(), pr.From.ID, pr.ID, fields)
	}
	if err := bw.Flush(); err != nil {
		for _, pr := range batch {
			pr.Written <- err
		}
		return err
	}
	if err := f.Sync(); err != nil {
		for _, pr := range batch {
			pr.Written <- err
		}
		return err
	}

	for _, pr := range batch {
		pr.Written <- nil
	}

	w.walLines += len(batch)
	if w.walLines >= checkpointAfter {
		return w.checkpoint(f)
	}
	return nil
}

// WriteAccept appends an acceptance entry to the WAL.
func (w *Writer) WriteAccept(reqID uint32) error {
	return w.writeResolution('A', reqID)
}

// WriteReject appends a rejection entry to the WAL.
func (w *Writer) WriteReject(reqID uint32) error {
	return w.writeResolution('X', reqID)
}

func (w *Writer) writeResolution(kind byte, reqID uint32) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.OpenFile(w.walPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "%c|%d|%d\n", kind, time.Now().Unix(), reqID)
	if err := f.Sync(); err != nil {
		return err
	}

	w.walLines++
	if w.walLines >= checkpointAfter {
		return w.checkpoint(f)
	}
	return nil
}

// checkpoint compacts the WAL into the canonical file, appends a K marker,
// and truncates the WAL. Rebuilds from disk — no in-memory state. Caller must hold w.mu.
func (w *Writer) checkpoint(walFile *os.File) error {
	if err := walFile.Sync(); err != nil {
		return err
	}

	pending := map[uint32]RawRequest{}
	if err := loadCanonical(w.canPath, pending); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := replayWALFile(walFile, pending); err != nil {
		return err
	}

	tmp := w.canPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	ts := time.Now().Unix()
	for _, r := range pending {
		fmt.Fprintf(bw, "R|%d|%d|%s|%d|%s\n", ts, r.ReqType, r.PeerID, r.ID, strings.Join(encodeRawFields(r.Fields), "|"))
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()

	if err := os.Rename(tmp, w.canPath); err != nil {
		os.Remove(tmp)
		return err
	}

	fmt.Fprintf(walFile, "K|%d\n", time.Now().Unix())
	if err := walFile.Sync(); err != nil {
		return err
	}
	if err := walFile.Truncate(0); err != nil {
		return err
	}
	w.walLines = 0
	return nil
}

// Replay loads the canonical file and replays all WAL entries after the last checkpoint marker.
func Replay(dir string) ([]RawRequest, error) {
	canPath := filepath.Join(dir, "requests")
	walPath := filepath.Join(dir, "requests.wal")

	pending := map[uint32]RawRequest{}

	if err := loadCanonical(canPath, pending); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("wal: load canonical: %w", err)
	}
	if err := replayWAL(walPath, pending); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("wal: replay wal: %w", err)
	}

	result := make([]RawRequest, 0, len(pending))
	for _, r := range pending {
		result = append(result, r)
	}
	return result, nil
}

// RawRequest is a pending request recovered from disk during startup replay.
type RawRequest struct {
	ID      uint32
	ReqType uint8
	PeerID  string
	Fields  []string
}

func loadCanonical(path string, out map[uint32]RawRequest) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		applyWALLine(sc.Text(), out)
	}
	return sc.Err()
}

func replayWAL(path string, out map[uint32]RawRequest) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return replayWALFile(f, out)
}

// replayWALFile replays a WAL file from the beginning, clearing out and
// re-applying only entries after the last K marker.
func replayWALFile(f *os.File, out map[uint32]RawRequest) error {
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return err
	}

	lastK := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "K|") {
			lastK = i
		}
	}

	for k := range out {
		delete(out, k)
	}
	for _, l := range lines[lastK+1:] {
		applyWALLine(l, out)
	}
	return nil
}

func applyWALLine(line string, out map[uint32]RawRequest) {
	if len(line) < 2 {
		return
	}
	switch line[0] {
	case 'R':
		// R|ts|reqtype|peer|id|field0|field1|...
		// SplitN with a high n to capture all trailing fields.
		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 5 {
			return
		}
		reqType, err := strconv.ParseUint(parts[2], 10, 8)
		if err != nil {
			return
		}
		id64, err := strconv.ParseUint(parts[4], 10, 32)
		if err != nil {
			return
		}
		var fields []string
		if len(parts) == 6 {
			fields = decodeFields(parts[5], reqType)
		}
		out[uint32(id64)] = RawRequest{
			ID:      uint32(id64),
			ReqType: uint8(reqType),
			PeerID:  parts[3],
			Fields:  fields,
		}
	case 'A', 'X':
		// A|ts|id  or  X|ts|id
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			return
		}
		id64, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			return
		}
		delete(out, uint32(id64))
	}
}

// encodeFields serializes the type-specific fields of an InboundRequest.
// New request types must add a case here and a matching case in decodeFields.
func encodeFields(req protocol.InboundRequest) (string, error) {
	switch r := req.(type) {
	case protocol.SymLinkRequest:
		return encodeField(r.Src) + "|" + encodeField(r.Dst), nil
	default:
		return "", fmt.Errorf("unknown InboundRequest type %T", req)
	}
}

// decodeFields splits and decodes the trailing field section of an R entry.
func decodeFields(raw string, reqType uint64) []string {
	switch uint8(reqType) {
	case protocol.MsgSymLinkRequest:
		parts := strings.SplitN(raw, "|", 2)
		out := make([]string, len(parts))
		for i, p := range parts {
			out[i] = decodeField(p)
		}
		return out
	default:
		return nil
	}
}

func encodeField(s string) string {
	return strings.ReplaceAll(s, "|", "%7C")
}

func decodeField(s string) string {
	return strings.ReplaceAll(s, "%7C", "|")
}

// encodeRawFields re-encodes already-decoded field values for writing to disk.
func encodeRawFields(fields []string) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = encodeField(f)
	}
	return out
}
