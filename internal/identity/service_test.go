package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewServiceRejectsUnusablePrivateKey(t *testing.T) {
	cases := map[string][]byte{
		// A crash between creating the file and writing the key.
		"empty":     {},
		"truncated": make([]byte, 40),
		// OpenSSH uses the same file name for its own key format.
		"OpenSSH format": []byte(
			"-----BEGIN OPENSSH PRIVATE KEY-----\n" +
				"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMw\n" +
				"-----END OPENSSH PRIVATE KEY-----\n",
		),
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := filepath.Join(home, ".lnn")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "id_ed25519")
			if err := os.WriteFile(path, key, 0600); err != nil {
				t.Fatal(err)
			}

			// ed25519 panics when signing with a key of any other size,
			// which would happen at the first handshake.
			if _, err := NewService(); err == nil {
				t.Fatalf("loaded a %d-byte private key without error",
					len(key))
			}
		})
	}
}
