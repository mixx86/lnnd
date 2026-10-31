package store

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

type walLayout struct {
	cp1 int64
	cp2 int64
	eof int64
}

func testEntry(op opKind, fields []byte) []byte {
	entryLength := uint32(entryHeaderLen + len(fields))
	header := encodeEntryHeader(op, entryLength)

	entry := make([]byte, 0, entryLength)
	entry = append(entry, header[:]...)
	entry = append(entry, fields...)
	return entry
}

func testCheckpoint(entries ...[]byte) []byte {
	var checksum uint32
	for _, entry := range entries {
		checksum += calculateChecksum(entry)
	}

	fields := make([]byte, checkpointChecksumLen)
	binary.BigEndian.PutUint32(fields, checksum)
	return testEntry(opCheckpoint, fields)
}

func writeTestWAL(t *testing.T, path string) walLayout {
	t.Helper()

	entry1 := testEntry(opAccepted, []byte{0, 0, 0, 1})
	entry2 := testEntry(opAccepted, []byte{0, 0, 0, 2})
	initialCheckpoint := testCheckpoint()
	checkpoint1 := testCheckpoint(entry1)
	checkpoint2 := testCheckpoint(entry2)

	cp1 := int64(fileHeaderLen + len(initialCheckpoint) + len(entry1))
	cp2 := cp1 + int64(len(checkpoint1)+len(entry2))
	eof := cp2 + int64(len(checkpoint2))

	var header [fileHeaderLen]byte
	binary.BigEndian.PutUint32(header[:fileHeaderMagicLen], fileHeaderMagic)
	binary.BigEndian.PutUint64(
		header[fileHeaderMagicLen:fileHeaderMagicLen+fileHeaderLCOLen],
		uint64(cp2),
	)
	binary.BigEndian.PutUint32(
		header[fileHeaderMagicLen+fileHeaderLCOLen:fileHeaderLen],
		2,
	)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	for _, part := range [][]byte{
		header[:],
		initialCheckpoint,
		entry1,
		checkpoint1,
		entry2,
		checkpoint2,
	} {
		if _, err := f.Write(part); err != nil {
			t.Fatal(err)
		}
	}

	return walLayout{cp1: cp1, cp2: cp2, eof: eof}
}

func runRequestsFileRecovery(t *testing.T, path string) {
	t.Helper()

	reqFile, err := OpenRequestsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reqFile.file.Close()

	if err, recoverable := reqFile.checkEntries(); err != nil {
		if recoverable {
			t.Fatalf(
				"unexpected recoverable requests entry in test WAL: %v",
				err,
			)
		}
		if err := reqFile.removeTail(); err != nil {
			t.Fatal(err)
		}
	}
}

func readFileLCO(t *testing.T, path string) uint64 {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var buf [fileHeaderLCOLen]byte
	if _, err := f.ReadAt(buf[:], fileHeaderMagicLen); err != nil {
		t.Fatal(err)
	}

	return binary.BigEndian.Uint64(buf[:])
}

func assertWALState(t *testing.T, path string, wantLCO uint64, wantSize int64) {
	t.Helper()

	if got := readFileLCO(t, path); got != wantLCO {
		t.Fatalf("lco: got %#x, want %#x", got, wantLCO)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Size(); got != wantSize {
		t.Fatalf("size: got %#x, want %#x", got, wantSize)
	}
}

func TestRequestsFileRecoveryRepairsMisalignedLCO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests")
	layout := writeTestWAL(t, path)

	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&RequestsFile{file: f}).setLco(uint64(layout.cp1 + 1)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	runRequestsFileRecovery(t, path)

	assertWALState(t, path, uint64(layout.cp2), layout.eof)
}

func TestRequestsFileRecoveryRollsBackTruncatedFinalCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests")
	layout := writeTestWAL(t, path)

	if err := os.Truncate(path, layout.cp2+2); err != nil {
		t.Fatal(err)
	}

	runRequestsFileRecovery(t, path)

	assertWALState(t, path, uint64(layout.cp1), layout.cp1+checkpointLen)
}

func TestRequestsFileRecoveryTruncatesGarbageAfterCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests")
	layout := writeTestWAL(t, path)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0xaa, 0xbb, 0xcc, 0xdd}); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	runRequestsFileRecovery(t, path)

	assertWALState(t, path, uint64(layout.cp2), layout.eof)
}
