package ipc

import (
	"testing"
)

func TestParseRequest_valid(t *testing.T) {
	data := []byte(
		`{"op":"link","type":"symreq","src":"192.168.1.34://path/to/file","dst":"./link"}`,
	)
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Op != "link" {
		t.Errorf("Op: got %q, want %q", req.Op, "link")
	}
	if req.Type != "symreq" {
		t.Errorf("Type: got %q, want %q", req.Type, "symreq")
	}
	// ParseRequest only deserializes; Src stays untouched until Validate runs.
	if req.Src != "192.168.1.34://path/to/file" {
		t.Errorf("Src: got %q, want %q", req.Src, "192.168.1.34://path/to/file")
	}
	if req.Dst != "./link" {
		t.Errorf("Dst: got %q, want %q", req.Dst, "./link")
	}
	if req.IP != "" {
		t.Errorf("IP: got %q, want it empty before Validate", req.IP)
	}
}

func TestParseRequest_ignoresIPField(t *testing.T) {
	// IP is derived by Validate; a client must not be able to set it directly.
	data := []byte(
		`{"op":"link","type":"symreq","src":"192.168.1.34://f","ip":"10.0.0.1"}`,
	)
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.IP != "" {
		t.Errorf("IP: got %q, want it ignored", req.IP)
	}
}

func TestParseRequest_invalidJSON(t *testing.T) {
	_, err := ParseRequest([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestValidate_valid(t *testing.T) {
	req := &Request{Src: "192.168.1.34://path/to/file"}
	if err := Validate(req); err != nil {
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
	if err := Validate(req); err != nil {
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
	if err := Validate(req); err == nil {
		t.Fatal("expected error for empty Src")
	}
}

func TestValidate_missingScheme(t *testing.T) {
	req := &Request{Src: "192.168.1.34/path/to/file"}
	if err := Validate(req); err == nil {
		t.Fatal("expected error for missing ://")
	}
}

func TestValidate_invalidIP(t *testing.T) {
	req := &Request{Src: "notanip://path/to/file"}
	if err := Validate(req); err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestValidate_hostnameRejected(t *testing.T) {
	req := &Request{Src: "example.com://path/to/file"}
	if err := Validate(req); err == nil {
		t.Fatal("expected error: only IP addresses are accepted, not hostnames")
	}
}

func TestValidate_pathNormalized(t *testing.T) {
	req := &Request{Src: "192.168.1.34://path/../path/to/file"}
	if err := Validate(req); err != nil {
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
	if err := Validate(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "/etc/passwd" {
		t.Errorf("path not rooted: got %q, want %q", req.Src, "/etc/passwd")
	}
}

func TestValidate_emptyPathBecomesRoot(t *testing.T) {
	req := &Request{Src: "192.168.1.34://"}
	if err := Validate(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "/" {
		t.Errorf("Src: got %q, want %q", req.Src, "/")
	}
}

func TestResponseEncode_ok(t *testing.T) {
	data, err := Response{OK: true}.Encode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(data), `{"ok":true}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestResponseEncode_error(t *testing.T) {
	data, err := Response{OK: false, Err: "boom"}.Encode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := string(data), `{"ok":false,"err":"boom"}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
