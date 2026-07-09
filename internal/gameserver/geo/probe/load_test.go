package probe

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
)

func TestLoadEngine(t *testing.T) {
	t.Run("loads an L2OFF region and leaves the rest null", func(t *testing.T) {
		dir := t.TempDir()
		writeFlatL2OFF(t, dir, engine.TileXMin, engine.TileYMin, 80)

		e, err := LoadEngine(dir, L2OFF)
		if err != nil {
			t.Fatalf("LoadEngine(): %v", err)
		}

		loadedX, loadedY := engine.WorldX(0), engine.WorldY(0)
		if got := e.Height(loadedX, loadedY, 0); got != 80 {
			t.Errorf("Height() in loaded region = %d, want 80", got)
		}

		if got := e.Height(engine.WorldXMax, engine.WorldYMax, 4321); got != 4321 {
			t.Errorf("Height() in unloaded region = %d, want unchanged worldZ 4321", got)
		}
	})

	t.Run("loads an L2J region", func(t *testing.T) {
		dir := t.TempDir()
		writeFlatL2J(t, dir, engine.TileXMin, engine.TileYMin, -40)

		e, err := LoadEngine(dir, L2J)
		if err != nil {
			t.Fatalf("LoadEngine(): %v", err)
		}

		x, y := engine.WorldX(0), engine.WorldY(0)
		if got := e.Height(x, y, 0); got != -40 {
			t.Errorf("Height() = %d, want -40", got)
		}
	})

	t.Run("no region files loads an all-null engine", func(t *testing.T) {
		e, err := LoadEngine(t.TempDir(), L2OFF)
		if err != nil {
			t.Fatalf("LoadEngine(): %v", err)
		}
		if got := e.Height(engine.WorldXMin, engine.WorldYMin, 999); got != 999 {
			t.Errorf("Height() = %d, want unchanged worldZ 999", got)
		}
	})

	t.Run("unknown geo type errors", func(t *testing.T) {
		if _, err := LoadEngine(t.TempDir(), "bogus"); err == nil {
			t.Fatal("LoadEngine() error = nil, want error")
		}
	})

	t.Run("corrupt region file errors", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "16_10_conv.dat")
		if err := os.WriteFile(path, []byte("too short"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadEngine(dir, L2OFF); err == nil {
			t.Fatal("LoadEngine() error = nil, want error")
		}
	})
}

// writeFlatL2OFF writes a full-size L2OFF region file, every block a flat
// floor at height, at the given tile coordinates.
func writeFlatL2OFF(t *testing.T, dir string, tileX, tileY int, height int16) {
	t.Helper()

	buf := make([]byte, 18+block.RegionBlockCount*6)
	off := 18
	for i := 0; i < block.RegionBlockCount; i++ {
		binary.LittleEndian.PutUint16(buf[off:], 0) // flat type
		binary.LittleEndian.PutUint16(buf[off+2:], uint16(height))
		binary.LittleEndian.PutUint16(buf[off+4:], 0) // dummy
		off += 6
	}

	path := filepath.Join(dir, fileNameFor(L2OFF, tileX, tileY))
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeFlatL2J writes a full-size L2J region file, every block a flat floor
// at height, at the given tile coordinates.
func writeFlatL2J(t *testing.T, dir string, tileX, tileY int, height int16) {
	t.Helper()

	buf := make([]byte, block.RegionBlockCount*3)
	off := 0
	for i := 0; i < block.RegionBlockCount; i++ {
		buf[off] = 0 // flat type
		binary.LittleEndian.PutUint16(buf[off+1:], uint16(height))
		off += 3
	}

	path := filepath.Join(dir, fileNameFor(L2J, tileX, tileY))
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fileNameFor(geoType GeoType, tileX, tileY int) string {
	return filepath.Base(regionPath("", geoType, tileX, tileY))
}
