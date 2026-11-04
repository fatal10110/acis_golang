package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBugReportLine(t *testing.T) {
	// Geodata cell (2048*3 + 8*5 + 2, 2048 + 8*7 + 6): region 3 and 1 past
	// the first, block 5, 7 in it, cell 2, 6 in that.
	x := WorldXMin + (2048*3+8*5+2)*16 + 9
	y := WorldYMin + (2048+8*7+6)*16
	got := BugReport(x, y, -3120, "stuck; behind the wall")
	want := "19;11;5;7;2;6;-3120;stuck: behind the wall\r\n"
	if TileXMin != 16 || TileYMin != 10 {
		t.Fatalf("tile origin %d_%d, the want line assumes 16_10", TileXMin, TileYMin)
	}
	if got != want {
		t.Fatalf("BugReport() = %q, want %q", got, want)
	}
}

func TestBugLogAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, BugLogFile)
	if err := os.WriteFile(path, []byte("earlier\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log := NewBugLog(dir)
	if err := log.Add(WorldXMin, WorldYMin, 7, "a"); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "earlier\r\n16;10;0;0;0;0;7;a\r\n"; string(data) != want {
		t.Fatalf("file = %q, want %q", data, want)
	}
}
