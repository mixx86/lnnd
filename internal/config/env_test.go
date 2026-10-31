package config

import "testing"

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
