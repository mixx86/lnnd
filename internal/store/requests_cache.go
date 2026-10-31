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
	cacheReqIDLen = 4
	cacheTagLen   = 2
	cacheEntryLen = cacheReqIDLen + cacheTagLen
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
	completeSize := size - (size % cacheEntryLen)

	var buf [cacheEntryLen]byte

	for offset := int64(0); offset < completeSize; offset += cacheEntryLen {
		if _, err := c.file.ReadAt(buf[:], offset); err != nil {
			return fmt.Errorf(
				"reading cache at offset %d: %w",
				offset,
				err,
			)
		}

		reqID := binary.BigEndian.Uint32(buf[:cacheReqIDLen])
		tag := binary.BigEndian.Uint16(buf[cacheReqIDLen:])

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
		sizeEntries := ((len(c.entries)) * cacheEntryLen)
		buf := make([]byte, sizeEntries)
		counter := 1
		for cache_reqID, cache_tag := range c.entries {
			offset := (counter * cacheEntryLen) - cacheEntryLen
			binary.BigEndian.PutUint32(
				buf[offset:offset+cacheReqIDLen],
				cache_reqID,
			)
			binary.BigEndian.PutUint16(
				buf[offset+cacheReqIDLen:offset+cacheEntryLen],
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
