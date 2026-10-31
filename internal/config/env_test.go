package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequestsQueueMaxBytes(t *testing.T) {
	const mib = 1024 * 1024
	cases := []struct {
		name     string
		override string
		maxMem   string
		want     int64
	}{
		{"override", "8MB", "", 8 * mib},
		{"derived from default max mem", "", "", 16 * mib},
		{"derived from max mem", "", "128MB", 32 * mib},
		{"invalid override", "abc", "", 16 * mib},
		{"zero override", "0", "", 16 * mib},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("LNND_REQ_QUEUE_MAX_BYTES", c.override)
			t.Setenv("LNND_MAX_MEM", c.maxMem)
			if got := RequestsQueueMaxBytes(); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}

func TestEnsureConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lnn", "lnnd.conf")

	created, err := EnsureConfig(path, []byte("A=1\n"))
	if err != nil || !created {
		t.Fatalf("first call: created=%v err=%v", created, err)
	}

	created, err = EnsureConfig(path, []byte("A=2\n"))
	if err != nil || created {
		t.Fatalf("second call: created=%v err=%v", created, err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A=1\n" {
		t.Errorf("existing config overwritten: %q", got)
	}
}
