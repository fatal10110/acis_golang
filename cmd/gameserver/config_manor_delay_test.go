package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoadManorDelay pins the seed purchase reuse delay to server.properties
// ManorTime and its shipped default of 3000 ms.
func TestLoadManorDelay(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.properties")
	set := filepath.Join(dir, "set.properties")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(set, []byte("ManorTime = 250\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadManorDelay(gameServerPaths{ConfigPath: empty}); err != nil || got != manorDelay(3*time.Second) {
		t.Fatalf("loadManorDelay(default) = %v, %v; want 3s", got, err)
	}
	if got, err := loadManorDelay(gameServerPaths{ConfigPath: set}); err != nil || got != manorDelay(250*time.Millisecond) {
		t.Fatalf("loadManorDelay(set) = %v, %v; want 250ms", got, err)
	}
}
