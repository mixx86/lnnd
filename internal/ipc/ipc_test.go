package ipc

import (
	"encoding/json"
	"errors"
	"io"
	app "lnn/internal"
	"net"
	"testing"

	"github.com/google/uuid"
)

func parseLinkRequest(t *testing.T, data []byte) *LinkRequest {
	t.Helper()
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req, ok := msg.(*LinkRequest)
	if !ok {
		t.Fatalf("got %T, want *LinkRequest", msg)
	}
	return req
}

func sendSymlink(ip, src string) *LinkRequest {
	return &LinkRequest{
		Request: Request{Header: Header{Op: "send"}, Kind: "symlink", IP: ip},
		Src:     src,
	}
}

func TestParseMessage_valid(t *testing.T) {
	req := parseLinkRequest(t, []byte(
		`{"type":"request","op":"send","kind":"symlink",`+
			`"ip":"192.168.1.34","src":"path/to/file","dst":"./link"}`,
	))
	if req.Op != "send" {
		t.Errorf("Op: got %q, want %q", req.Op, "send")
	}
	if req.Kind != "symlink" {
		t.Errorf("Kind: got %q, want %q", req.Kind, "symlink")
	}
	if req.IP != "192.168.1.34" {
		t.Errorf("IP: got %q, want %q", req.IP, "192.168.1.34")
	}
	// ParseMessage only deserializes; Src stays untouched until
	// ValidateLinkRequest runs.
	if req.Src != "path/to/file" {
		t.Errorf("Src: got %q, want %q", req.Src, "path/to/file")
	}
	if req.Dst != "./link" {
		t.Errorf("Dst: got %q, want %q", req.Dst, "./link")
	}
}

func TestParseMessage_invalidJSON(t *testing.T) {
	_, err := ParseMessage([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// HandleConn answers ErrUnknownType only when the error wraps
// errUnknownType; any other error is reported as ErrInvalidMessage.
func TestParseMessage_unknownType(t *testing.T) {
	for _, data := range []string{
		`{"type":"symreq","op":"link"}`,
		`{"op":"link"}`,
	} {
		_, err := ParseMessage([]byte(data))
		if !errors.Is(err, errUnknownType) {
			t.Errorf("%s: got %v, want errUnknownType", data, err)
		}
	}
}

// A request that names no known op or kind must get an answer saying which
// one is unknown, not a failed check of link fields it never meant to carry.
func TestHandleMessage_unrecognizedRequest(t *testing.T) {
	tests := []struct {
		name string
		data string
		want Code
	}{
		{
			name: "unknown kind",
			data: `{"type":"request","op":"send","kind":"sym"}`,
			want: ErrInvalidRequest,
		},
		{
			name: "unknown op with a link kind",
			data: `{"type":"request","op":"accept","kind":"symlink"}`,
			want: ErrUnknownOp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			client, server := net.Pipe()
			defer client.Close()
			go func() {
				HandleMessage(msg, uuid.UUID{}, server, app.Services{}, nil)
				server.Close()
			}()

			data, err := io.ReadAll(client)
			if err != nil {
				t.Fatalf("reading the response: %v", err)
			}
			var res Response
			if err := json.Unmarshal(data, &res); err != nil {
				t.Fatalf("decoding response %q: %v", data, err)
			}
			if res.Code != tt.want {
				t.Errorf(
					"code: got %d, want %d (%s)",
					res.Code,
					tt.want,
					res.Err,
				)
			}
		})
	}
}

func TestValidate_valid(t *testing.T) {
	req := sendSymlink("192.168.1.34", "path/to/file")
	if err := ValidateLinkRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "/path/to/file" {
		t.Errorf("Src: got %q, want %q", req.Src, "/path/to/file")
	}
}

func TestValidate_ipv6(t *testing.T) {
	req := sendSymlink("::1", "/path/to/file")
	if err := ValidateLinkRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_emptySrc(t *testing.T) {
	req := sendSymlink("192.168.1.34", "")
	if err := ValidateLinkRequest(req); err == nil {
		t.Fatal("expected error for empty Src")
	}
}

func TestValidate_missingIP(t *testing.T) {
	req := sendSymlink("", "/path/to/file")
	if err := ValidateLinkRequest(req); err == nil {
		t.Fatal("expected error for missing IP")
	}
}

func TestValidate_invalidIP(t *testing.T) {
	req := sendSymlink("notanip", "/path/to/file")
	if err := ValidateLinkRequest(req); err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestValidate_hostnameRejected(t *testing.T) {
	req := sendSymlink("example.com", "/path/to/file")
	if err := ValidateLinkRequest(req); err == nil {
		t.Fatal("expected error: only IP addresses are accepted, not hostnames")
	}
}

func TestValidate_pathNormalized(t *testing.T) {
	req := sendSymlink("192.168.1.34", "path/../path/to/file")
	if err := ValidateLinkRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "/path/to/file" {
		t.Errorf(
			"path not normalized: got %q, want %q",
			req.Src,
			"/path/to/file",
		)
	}
}

func TestValidate_pathCannotEscapeRoot(t *testing.T) {
	req := sendSymlink("192.168.1.34", "../../etc/passwd")
	if err := ValidateLinkRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "/etc/passwd" {
		t.Errorf("path not rooted: got %q, want %q", req.Src, "/etc/passwd")
	}
}

func TestResponseEncode_ok(t *testing.T) {
	data, err := Response{Code: OK}.Encode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(data), `{"code":0}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResponseEncode_error(t *testing.T) {
	data, err := Response{Code: ErrInvalidMessage, Err: "boom"}.Encode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(data), `{"code":1,"err":"boom"}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
