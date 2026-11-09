package outbox

import (
	"sync"

	"github.com/google/uuid"

	"lnn/internal/protocol"
)

const (
	// Window bounds memory to ~2 MB per peer in the worst case (8 KB
	// requests) while staying far below the tag space, so a tag is never
	// reused while its message can still be asked for.
	Window = 256

	// maxUnflushed caps records not yet fsynced; restartTagSkip must be at
	// least as large, so tags that reached a peer but not the disk are
	// never handed out again after a crash.
	maxUnflushed   = 1024
	restartTagSkip = maxUnflushed

	compactAfterBytes = 1 << 20
)

type window struct {
	lastTag     uint32
	unackedTags []uint32
	frames      map[uint32]protocol.Frame
}

type Outbox struct {
	mu    sync.Mutex
	peers map[uuid.UUID]*window

	log *logFile

	pending  [][]byte
	inflight int
	drained  *sync.Cond
	wake     chan struct{}
	closing  bool
	stopped  chan struct{}
}

// Put assigns the next tag for peer and keeps the frame until the peer
// confirms it. When the window is full the oldest frame is dropped: a
// peer that has not confirmed 256 requests is gone or stuck, and asking
// for a dropped tag yields ErrUnknownTag instead of a silent hang.
func (o *Outbox) Put(peer uuid.UUID, body protocol.Payload) protocol.Frame {
	o.mu.Lock()
	defer o.mu.Unlock()

	for len(o.pending)+o.inflight >= maxUnflushed && !o.closing {
		o.drained.Wait()
	}

	w := o.window(peer)
	tag := w.nextTag()

	if len(w.unackedTags) >= Window {
		oldest := w.unackedTags[0]
		w.unackedTags = w.unackedTags[1:]
		delete(w.frames, oldest)
		o.enqueue(encodeRemoved(peer, oldest))
	}

	frame := protocol.Frame{Body: body}.WithTag(tag)
	w.frames[tag] = frame
	w.unackedTags = append(w.unackedTags, tag)
	o.enqueue(encodeSent(peer, frame))

	return frame
}

func (o *Outbox) Get(peer uuid.UUID, tag uint32) (protocol.Frame, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	w, ok := o.peers[peer]
	if !ok {
		return protocol.Frame{}, false
	}
	frame, ok := w.frames[tag]
	return frame, ok
}

// Ack marks the end of the exchange the tag carried, not that the peer
// accepted the message: it is called for both OK and ERROR, since either
// way the peer has answered and will never ask for the frame again.
func (o *Outbox) Ack(peer uuid.UUID, tag uint32) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	w, ok := o.peers[peer]
	if !ok {
		return false
	}
	if _, ok := w.frames[tag]; !ok {
		return false
	}

	delete(w.frames, tag)
	for i, t := range w.unackedTags {
		if t == tag {
			w.unackedTags = append(w.unackedTags[:i], w.unackedTags[i+1:]...)
			break
		}
	}
	o.enqueue(encodeRemoved(peer, tag))
	return true
}

func (o *Outbox) window(peer uuid.UUID) *window {
	w, ok := o.peers[peer]
	if !ok {
		w = &window{frames: make(map[uint32]protocol.Frame)}
		o.peers[peer] = w
	}
	return w
}

// nextTag wraps around and skips 0, which marks untagged frames, as well
// as any tag still waiting for confirmation.
func (w *window) nextTag() uint32 {
	for {
		w.lastTag++
		if w.lastTag == 0 {
			continue
		}
		if _, used := w.frames[w.lastTag]; !used {
			return w.lastTag
		}
	}
}

func (o *Outbox) enqueue(record []byte) {
	o.pending = append(o.pending, record)
	select {
	case o.wake <- struct{}{}:
	default:
	}
}
