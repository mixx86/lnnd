package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequestsCacheAppendsOnlyNewEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.cache")
	c, err := OpenRequestsCache(path, 2)
	if err != nil {
		t.Fatal(err)
	}

	puts := []struct {
		reqID, tag uint32
		flush      bool
	}{
		{1, 10, false},
		{2, 20, true},
		{3, 30, true},
		{4, 40, false},
		{5, 50, true},
	}
	for _, p := range puts {
		if err := c.Put(p.reqID, p.tag, p.flush); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(len(puts) * cacheEntryLen); info.Size() != want {
		t.Fatalf("cache size %d, want %d (one slot per Put)",
			info.Size(), want)
	}

	c, err = OpenRequestsCache(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, p := range puts {
		if tag, ok := c.GetTag(p.reqID); !ok || tag != p.tag {
			t.Fatalf("req %d: got tag %d ok=%v, want %d",
				p.reqID, tag, ok, p.tag)
		}
	}
}
