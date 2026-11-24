package store

import (
	"encoding/binary"
	"fmt"
	"io"

	"os"
)

//  slot:
//    [lcl_req_id:4]
//    [tag:4]

const (
	cacheLCLReqIDLen = 4
	cacheTagLen      = 4
	cacheEntryLen    = cacheLCLReqIDLen + cacheTagLen
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
		entries: make(map[uint32]uint32),
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

		lclReqID := binary.BigEndian.Uint32(buf[:cacheLCLReqIDLen])
		tag := binary.BigEndian.Uint32(buf[cacheLCLReqIDLen:])

		c.entries[lclReqID] = tag
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

// Put only appends entries added since the last flush: rewriting the
// whole map every time made the file grow quadratically. load keeps the
// last slot per lcl_req_id, so a repeated lcl_req_id is harmless.
func (c *RequestsCache) Put(lclReqID uint32, tag uint32, flush bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[lclReqID] = tag

	var slot [cacheEntryLen]byte
	binary.BigEndian.PutUint32(slot[:cacheLCLReqIDLen], lclReqID)
	binary.BigEndian.PutUint32(slot[cacheLCLReqIDLen:], tag)
	c.unflushed = append(c.unflushed, slot[:]...)

	if !flush {
		return nil
	}
	if _, err := c.file.Write(c.unflushed); err != nil {
		return err
	}
	c.unflushed = c.unflushed[:0]
	return c.file.Sync()
}

func (c *RequestsCache) GetTag(lclReqID uint32) (uint32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tag, ok := c.entries[lclReqID]
	return tag, ok
}

func (c *RequestsCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.file.Close()
}
