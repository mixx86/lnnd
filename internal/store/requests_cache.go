package store

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

func OpenRequestsCache(path string, flushEvery int) (*RequestsCache, error) {
	if flushEvery <= 0 {
		return nil, fmt.Errorf("invalid cache flush interval: %d", flushEvery)
	}

	f, err := os.OpenFile(
		path,
		os.O_RDWR|os.O_CREATE|os.O_APPEND,
		0644,
	)
	if err != nil {
		return nil, err
	}

	c := &RequestsCache{
		file:       f,
		entries:    make(map[uint32]uint16),
		flushEvery: flushEvery,
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

func (c *RequestsCache) Put(reqID uint32, tag uint16) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var buf [CACHE_ENTRY_LEN]byte

	binary.BigEndian.PutUint32(
		buf[:CACHE_REQ_ID_LEN],
		reqID,
	)

	binary.BigEndian.PutUint16(
		buf[CACHE_REQ_ID_LEN:],
		tag,
	)

	if _, err := c.file.Write(buf[:]); err != nil {
		return err
	}

	c.entries[reqID] = tag
	c.pending++

	if c.pending >= c.flushEvery {
		if err := c.file.Sync(); err != nil {
			return err
		}

		c.pending = 0
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
