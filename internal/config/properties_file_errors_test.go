package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadFileRefusesMissingAndMalformedFiles pins what a configuration file
// that cannot be read does here: the load fails with an error naming the
// file, and returns no properties. The reference logs the error and goes on
// with an empty set, so every key takes its default; that difference is
// tracked in #4772 (owner decision), not pinned as correct.
func TestLoadFileRefusesMissingAndMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.properties")
	p, err := LoadFile(missing)
	if p != nil || !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), missing) {
		t.Errorf("LoadFile(missing) = %v, %v; want nil and a not-exist error naming the file", p, err)
	}

	malformed := filepath.Join(dir, "bad.properties")
	if err := os.WriteFile(malformed, []byte("Key = \\u12\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err = LoadFile(malformed)
	if p != nil || err == nil || !strings.Contains(err.Error(), malformed) {
		t.Errorf("LoadFile(malformed) = %v, %v; want nil and an error naming the file", p, err)
	}
}
