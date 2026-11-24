package recovery

import (
	"encoding/binary"
	"errors"
	"os"
	"sync"

	"github.com/google/uuid"

	"lnn/internal/fsutil"
)

// Record format: [peer_uuid:16][lcl_req_id:4][tag:4]
const (
	peerLen     = 16
	lclReqIDLen = 4
	tagLen      = 4
	recordLen   = peerLen + lclReqIDLen + tagLen
)

type Entry struct {
	Peer     uuid.UUID
	LCLReqID uint32
	Tag      uint32
}

type key struct {
	peer uuid.UUID
	tag  uint32
}

// Pending lives in its own package so network can resolve entries and
// store can add them without importing each other. It is persisted
// because the damaged WAL region is truncated as soon as it is recorded
// here: a second crash must not forget what is still owed by peers.
type Pending struct {
	mu      sync.Mutex
	path    string
	entries map[key]Entry
}

func Open(path string) (*Pending, error) {
	p := &Pending{path: path, entries: make(map[key]Entry)}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}

	// Files are only ever replaced atomically, so a partial record means
	// foreign damage; keep whatever complete records precede it.
	for len(data) >= recordLen {
		e := Entry{
			Peer: uuid.UUID(data[:peerLen]),
			LCLReqID: binary.BigEndian.Uint32(
				data[peerLen : peerLen+lclReqIDLen],
			),
			Tag: binary.BigEndian.Uint32(
				data[peerLen+lclReqIDLen : recordLen],
			),
		}
		p.entries[key{e.Peer, e.Tag}] = e
		data = data[recordLen:]
	}
	return p, nil
}

func (p *Pending) Add(entries ...Entry) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, e := range entries {
		p.entries[key{e.Peer, e.Tag}] = e
	}
	return p.save()
}

// Resolve removes the entry once the peer has either resent the request
// or answered ErrUnknownTag; both end the recovery.
func (p *Pending) Resolve(peer uuid.UUID, tag uint32) (Entry, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	k := key{peer, tag}
	e, ok := p.entries[k]
	if !ok {
		return Entry{}, false, nil
	}
	delete(p.entries, k)
	return e, true, p.save()
}

func (p *Pending) ForPeer(peer uuid.UUID) []Entry {
	p.mu.Lock()
	defer p.mu.Unlock()

	var out []Entry
	for k, e := range p.entries {
		if k.peer == peer {
			out = append(out, e)
		}
	}
	return out
}

func (p *Pending) save() error {
	buf := make([]byte, 0, len(p.entries)*recordLen)
	for _, e := range p.entries {
		var rec [recordLen]byte
		copy(rec[:peerLen], e.Peer[:])
		binary.BigEndian.PutUint32(rec[peerLen:peerLen+lclReqIDLen], e.LCLReqID)
		binary.BigEndian.PutUint32(rec[peerLen+lclReqIDLen:], e.Tag)
		buf = append(buf, rec[:]...)
	}
	return fsutil.WriteFileAtomic(p.path, buf)
}
