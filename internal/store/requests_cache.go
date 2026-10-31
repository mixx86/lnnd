package store

import (
	"encoding/binary"
	"fmt"
	"io"

	"os"
)

//  slot:
//    [req_id:4]
//    [tag:2]
// file size = 16 + capacity*6

const (
	CACHE_REQ_ID_LEN = 4
	CACHE_TAG_LEN    = 2
	CACHE_ENTRY_LEN  = CACHE_REQ_ID_LEN + CACHE_TAG_LEN
)

func OpenRequestsCache(path string, flushEvery int) (*RequestsCache, error) {
	f, err := os.OpenFile(
		path,
		os.O_RDWR|os.O_CREATE,
		0644,
	)
	if err != nil {
		return nil, err
	}

	c := &RequestsCache{
		file:    f,
		entries: make(map[uint32]uint16),
	}

	if err := c.load(); err != nil {
		_ = f.Close()
		return nil, err
	}

	return c, nil
}

func (c *RequestsCache) load() error {
	info, err := c.file.Stat()
	if err != nil {
		return err
	}

	size := info.Size()
	completeSize := size - (size % CACHE_ENTRY_LEN)

	var buf [CACHE_ENTRY_LEN]byte

	for offset := int64(0); offset < completeSize; offset += CACHE_ENTRY_LEN {
		if _, err := c.file.ReadAt(buf[:], offset); err != nil {
			return fmt.Errorf(
				"reading cache at offset %d: %w",
				offset,
				err,
			)
		}

		reqID := binary.BigEndian.Uint32(buf[:CACHE_REQ_ID_LEN])
		tag := binary.BigEndian.Uint16(buf[CACHE_REQ_ID_LEN:])

		c.entries[reqID] = tag
	}

	if completeSize != size {
		if err := c.file.Truncate(completeSize); err != nil {
			return fmt.Errorf(
				"truncating incomplete cache entry: %w",
				err,
			)
		}
	}

	if _, err := c.file.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	return nil
}

func (c *RequestsCache) Put(reqID uint32, tag uint16, flush bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[reqID] = tag

	if flush == true {
		sizeEntries := ((len(c.entries)) * CACHE_ENTRY_LEN)
		buf := make([]byte, sizeEntries)
		counter := 1
		for cache_reqID, cache_tag := range c.entries {
			offset := (counter * CACHE_ENTRY_LEN) - CACHE_ENTRY_LEN
			binary.BigEndian.PutUint32(
				buf[offset:offset+CACHE_REQ_ID_LEN],
				cache_reqID,
			)
			binary.BigEndian.PutUint16(
				buf[offset+CACHE_REQ_ID_LEN:offset+CACHE_ENTRY_LEN],
				cache_tag,
			)
			counter += 1
		}

		if _, err := c.file.Write(buf[:]); err != nil {
			return err
		}
		if err := c.file.Sync(); err != nil {
			return err
		}
	}

	return nil
}

func (c *RequestsCache) GetTag(reqID uint32) (uint16, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tag, ok := c.entries[reqID]
	return tag, ok
}

func (c *RequestsCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.file.Close()
}
