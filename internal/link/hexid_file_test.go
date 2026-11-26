package link

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWriteHexIDFile writes the properties file the game server boots
// from, replacing an older one whole, with no temporary file left behind.
func TestWriteHexIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hexid.txt")
	if err := os.WriteFile(path, []byte("ServerID=1\nHexID=1\n#old content longer than the new one ............................................\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)
	if err := WriteHexIDFile(path, 2, []byte{0x80, 0x01}, now); err != nil {
		t.Fatalf("WriteHexIDFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "#the hexID to auth into login\n#Wed Mar 04 05:06:07 UTC 2026\nServerID=2\nHexID=-7fff\n"
	if string(data) != want {
		t.Fatalf("hexid file =\n%s\nwant\n%s", data, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("hexid file stat = %v, %v; want mode 0644", info, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir holds %s (%v), want only hexid.txt", strings.Join(names, ", "), err)
	}
}

// TestWriteHexIDFileFailureReportsError fails to write into a missing
// directory.
func TestWriteHexIDFileFailureReportsError(t *testing.T) {
	if err := WriteHexIDFile(filepath.Join(t.TempDir(), "missing", "hexid.txt"), 2, []byte{1}, time.Now()); err == nil {
		t.Fatal("WriteHexIDFile into a missing directory: want an error")
	}
}
