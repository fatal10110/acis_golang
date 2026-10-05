package logging

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRotatingFileKeepsWritingAfterRenameFailure forces the generation shift to fail
// (generation 1 is a non-empty directory, so neither the delete nor the rename can
// succeed) and checks that the sink still reopens generation 0, as the JUL
// FileHandler does when renameTo fails, instead of staying closed until restart.
func TestRotatingFileKeepsWritingAfterRenameFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "app_1.txt")
	if err := os.MkdirAll(filepath.Join(blocker, "keep"), 0o755); err != nil {
		t.Fatal(err)
	}

	rf, err := newRotatingFile(dir, "app_%g.txt", 10, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()

	if _, err := rf.Write([]byte("0123456789")); err != nil { // reaches limit, rotation's rename fails
		t.Fatalf("write that triggers a failed rename: %v", err)
	}
	if _, err := rf.Write([]byte("after\n")); err != nil {
		t.Fatalf("write after failed rename: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "app_0.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after\n" {
		t.Fatalf("generation 0 = %q, want %q (reopened and truncated like JUL)", got, "after\n")
	}
	if info, err := os.Stat(blocker); err != nil || !info.IsDir() {
		t.Fatalf("blocking generation 1 directory changed: info=%v err=%v", info, err)
	}
}
