package network

import "testing"

func TestTCPWithPort(t *testing.T) {
	tr := &TCPTransport{Port: "5771"}
	cases := map[string]string{
		"127.0.0.1":      "127.0.0.1:5771",
		"127.0.0.1:9000": "127.0.0.1:9000",
		"::1":            "[::1]:5771",
		"[::1]:9000":     "[::1]:9000",
	}
	for in, want := range cases {
		if got := tr.withPort(in); got != want {
			t.Errorf("withPort(%q) = %q, want %q", in, got, want)
		}
	}
}
