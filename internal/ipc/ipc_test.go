package ipc

import (
	"errors"
	"testing"
)

func parseRequest(t *testing.T, data []byte) *Request {
	t.Helper()
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req, ok := msg.(*Request)
	if !ok {
		t.Fatalf("got %T, want *Request", msg)
	}
	return req
}

func TestParseMessage_valid(t *testing.T) {
	req := parseRequest(t, []byte(
		`{"type":"request","op":"link","link":"sym",`+
			`"src":"192.168.1.34://path/to/file","dst":"./link"}`,
	))
	if req.Op != "link" {
		t.Errorf("Op: got %q, want %q", req.Op, "link")
	}
	if req.Link != "sym" {
		t.Errorf("Link: got %q, want %q", req.Link, "sym")
	}
	// ParseMessage only deserializes; Src stays untouched until
	// ValidateRequest runs.
	if req.Src != "192.168.1.34://path/to/file" {
		t.Errorf("Src: got %q, want %q", req.Src, "192.168.1.34://path/to/file")
	}
	if req.Dst != "./link" {
		t.Errorf("Dst: got %q, want %q", req.Dst, "./link")
	}
	if req.IP != "" {
		t.Errorf("IP: got %q, want it empty before ValidateRequest", req.IP)
	}
}

func TestParseMessage_ignoresIPField(t *testing.T) {
	// IP is derived by ValidateRequest; a client must not be able to set
	// it directly.
	req := parseRequest(t, []byte(
		`{"type":"request","op":"link","link":"sym",`+
			`"src":"192.168.1.34://f","ip":"10.0.0.1"}`,
	))
	if req.IP != "" {
		t.Errorf("IP: got %q, want it ignored", req.IP)
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

func TestValidate_valid(t *testing.T) {
	req := &Request{Src: "192.168.1.34://path/to/file"}
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.IP != "192.168.1.34" {
		t.Errorf("IP: got %q, want %q", req.IP, "192.168.1.34")
	}
	if req.Src != "/path/to/file" {
		t.Errorf("Src: got %q, want %q", req.Src, "/path/to/file")
	}
}

func TestValidate_ipv6(t *testing.T) {
	req := &Request{Src: "::1://path/to/file"}
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.IP != "::1" {
		t.Errorf("IP: got %q, want %q", req.IP, "::1")
	}
	if req.Src != "/path/to/file" {
		t.Errorf("Src: got %q, want %q", req.Src, "/path/to/file")
	}
}

func TestValidate_emptySrc(t *testing.T) {
	req := &Request{Src: ""}
	if err := ValidateRequest(req); err == nil {
		t.Fatal("expected error for empty Src")
	}
}

func TestValidate_missingScheme(t *testing.T) {
	req := &Request{Src: "192.168.1.34/path/to/file"}
	if err := ValidateRequest(req); err == nil {
		t.Fatal("expected error for missing ://")
	}
}

func TestValidate_invalidIP(t *testing.T) {
	req := &Request{Src: "notanip://path/to/file"}
	if err := ValidateRequest(req); err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestValidate_hostnameRejected(t *testing.T) {
	req := &Request{Src: "example.com://path/to/file"}
	if err := ValidateRequest(req); err == nil {
		t.Fatal("expected error: only IP addresses are accepted, not hostnames")
	}
}

func TestValidate_pathNormalized(t *testing.T) {
	req := &Request{Src: "192.168.1.34://path/../path/to/file"}
	if err := ValidateRequest(req); err != nil {
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
	req := &Request{Src: "192.168.1.34://../../etc/passwd"}
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "/etc/passwd" {
		t.Errorf("path not rooted: got %q, want %q", req.Src, "/etc/passwd")
	}
}

func TestValidate_emptyPathBecomesRoot(t *testing.T) {
	req := &Request{Src: "192.168.1.34://"}
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "/" {
		t.Errorf("Src: got %q, want %q", req.Src, "/")
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
