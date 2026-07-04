package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHexID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hexid(server 1).txt")
	if err := writeHexID(path, 1, "-7fff"); err != nil {
		t.Fatalf("writeHexID() unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hexid file: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("hexid file has %d lines, want 4:\n%s", len(lines), data)
	}
	if lines[0] != "#the hexID to auth into login" {
		t.Errorf("comment line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "#") {
		t.Errorf("date line = %q, want # prefix", lines[1])
	}
	if lines[2] != "ServerID=1" {
		t.Errorf("server id line = %q, want %q", lines[2], "ServerID=1")
	}
	if lines[3] != "HexID=-7fff" {
		t.Errorf("hex id line = %q, want %q", lines[3], "HexID=-7fff")
	}
}
