package ipc

import (
	"testing"
)

// --- ParseRequest ---

func TestParseRequest_valid(t *testing.T) {
	data := []byte(`{"op":"link","type":"sym","src":"192.168.1.34://path/to/file","dst":"./link"}`)
	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Op != "link" {
		t.Errorf("Op: got %q, want %q", req.Op, "link")
	}
	if req.Type != "sym" {
		t.Errorf("Type: got %q, want %q", req.Type, "sym")
	}
	if req.Dst != "./link" {
		t.Errorf("Dst: got %q, want %q", req.Dst, "./link")
	}
}

func TestParseRequest_invalidJSON(t *testing.T) {
	_, err := ParseRequest([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// --- Validate ---

func TestValidate_valid(t *testing.T) {
	req := &Request{Src: "192.168.1.34://path/to/file"}
	if err := Validate(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.IP != "192.168.1.34" {
		t.Errorf("IP: got %q, want %q", req.IP, "192.168.1.34")
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

func TestValidate_pathNormalized(t *testing.T) {
	req := &Request{Src: "192.168.1.34://path/../path/to/file"}
	if err := Validate(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Src != "192.168.1.34://path/to/file" {
		t.Errorf("path not normalized: got %q", req.Src)
	}
}
