package probe

import (
	"bytes"
	"encoding/binary"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/rs/zerolog"
)

// writeMixedL2OFF writes a full-size L2OFF region whose blocks cycle through
// flat, complex and multilayer layouts, block 0 a flat floor at height.
func writeMixedL2OFF(t *testing.T, dir string, tileX, tileY int, height int16) {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(tileX), uint64(tileY)))
	buf := make([]byte, 18, 18+block.RegionBlockCount*64)
	u16 := func(v uint16) { buf = binary.LittleEndian.AppendUint16(buf, v) }
	for i := range block.RegionBlockCount {
		switch i % 3 {
		case 0:
			u16(0x0000)
			u16(uint16(height))
			u16(0)
		case 1:
			u16(0x0040)
			for range block.CellCount {
				u16(uint16(rng.Uint32()))
			}
		default:
			u16(0x0001)
			for range block.CellCount {
				n := 1 + rng.IntN(3)
				u16(uint16(n))
				for range n {
					u16(uint16(rng.Uint32()))
				}
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, fileNameFor(L2OFF, tileX, tileY)), buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func storedBytes(t *testing.T, r *block.Region) []byte {
	t.Helper()
	var b bytes.Buffer
	if _, err := r.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestLoadRegionsCached covers the cache lifecycle at boot: build when
// missing, map when current, rebuild when stale or corrupt, and never mask
// a missing source with a cache built while it existed.
func TestLoadRegionsCached(t *testing.T) {
	regions := []Region{{X: engine.TileXMin, Y: engine.TileYMin}, {X: engine.TileXMin + 1, Y: engine.TileYMin}}
	x, y := engine.WorldX(0), engine.WorldY(0)
	setup := func(t *testing.T) (dir, cachePath string) {
		dir = t.TempDir()
		writeMixedL2OFF(t, dir, regions[0].X, regions[0].Y, 80)
		writeMixedL2OFF(t, dir, regions[1].X, regions[1].Y, 90)
		return dir, filepath.Join(t.TempDir(), "geodata.cache")
	}
	load := func(t *testing.T, dir, cachePath string) (*engine.Engine, string) {
		t.Helper()
		var logs bytes.Buffer
		e, err := LoadRegionsCached(dir, L2OFF, regions, cachePath, zerolog.New(&logs))
		if err != nil {
			t.Fatalf("LoadRegionsCached(): %v", err)
		}
		return e, logs.String()
	}

	t.Run("builds a missing cache, then maps it", func(t *testing.T) {
		dir, cachePath := setup(t)
		e, logs := load(t, dir, cachePath)
		if !strings.Contains(logs, "rebuilding") || !strings.Contains(logs, `"region_files":2`) {
			t.Fatalf("first boot log = %s, want a rebuild and region_files 2", logs)
		}
		if got := e.Height(x, y, 0); got != 80 {
			t.Fatalf("Height() = %d, want 80", got)
		}
		e, logs = load(t, dir, cachePath)
		if strings.Contains(logs, "rebuilding") {
			t.Fatalf("second boot log = %s, want the cache used as is", logs)
		}
		if got := e.Height(x, y, 0); got != 80 {
			t.Fatalf("mapped Height() = %d, want 80", got)
		}
	})

	t.Run("rebuilds when a source file changes", func(t *testing.T) {
		dir, cachePath := setup(t)
		load(t, dir, cachePath)
		writeMixedL2OFF(t, dir, regions[0].X, regions[0].Y, 120)
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(filepath.Join(dir, fileNameFor(L2OFF, regions[0].X, regions[0].Y)), later, later); err != nil {
			t.Fatal(err)
		}
		e, logs := load(t, dir, cachePath)
		if !strings.Contains(logs, "source file changed") {
			t.Fatalf("log = %s, want a stale-source rebuild", logs)
		}
		if got := e.Height(x, y, 0); got != 120 {
			t.Fatalf("Height() = %d, want the changed source's 120", got)
		}
	})

	for name, corrupt := range map[string]func(b []byte) []byte{
		"rebuilds a corrupt cache":   func(b []byte) []byte { b[len(b)-100] ^= 0x40; return b },
		"rebuilds a truncated cache": func(b []byte) []byte { return b[:len(b)/2] },
		"rebuilds a foreign file":    func(b []byte) []byte { b[0] = 'X'; return b },
		"rebuilds an older format": func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[8:], cacheVersion-1)
			return b
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, cachePath := setup(t)
			load(t, dir, cachePath)
			b, err := os.ReadFile(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cachePath, corrupt(b), 0o600); err != nil {
				t.Fatal(err)
			}
			e, logs := load(t, dir, cachePath)
			if !strings.Contains(logs, "rebuilding") {
				t.Fatalf("log = %s, want a rebuild", logs)
			}
			if got := e.Height(x, y, 0); got != 80 {
				t.Fatalf("Height() = %d, want 80", got)
			}
			if _, err := openCache(cachePath, L2OFF, mustStat(t, dir, regions)); err != nil {
				t.Fatalf("rebuilt cache: %v", err)
			}
		})
	}

	t.Run("a missing listed source fails boot despite a cache", func(t *testing.T) {
		dir, cachePath := setup(t)
		load(t, dir, cachePath)
		if err := os.Remove(filepath.Join(dir, fileNameFor(L2OFF, regions[1].X, regions[1].Y))); err != nil {
			t.Fatal(err)
		}
		_, err := LoadRegionsCached(dir, L2OFF, regions, cachePath, zerolog.Nop())
		if err == nil || !strings.Contains(err.Error(), "the server will not start") {
			t.Fatalf("LoadRegionsCached() error = %v, want the listed-region boot failure", err)
		}
	})

	t.Run("a different region list rebuilds", func(t *testing.T) {
		dir, cachePath := setup(t)
		load(t, dir, cachePath)
		var logs bytes.Buffer
		if _, err := LoadRegionsCached(dir, L2OFF, regions[:1], cachePath, zerolog.New(&logs)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(logs.String(), "rebuilding") {
			t.Fatalf("log = %s, want a rebuild for the new region list", logs.String())
		}
	})

	t.Run("an unwritable cache location loads the region files", func(t *testing.T) {
		dir, _ := setup(t)
		var logs bytes.Buffer
		e, err := LoadRegionsCached(dir, L2OFF, regions, filepath.Join(dir, "missing", "geodata.cache"), zerolog.New(&logs))
		if err != nil {
			t.Fatalf("LoadRegionsCached(): %v", err)
		}
		if !strings.Contains(logs.String(), "loading region files instead") {
			t.Fatalf("log = %s, want the heap fallback", logs.String())
		}
		if got := e.Height(x, y, 0); got != 80 {
			t.Fatalf("Height() = %d, want 80", got)
		}
	})
}

func mustStat(t *testing.T, dir string, regions []Region) []sourceFile {
	t.Helper()
	sources, err := statSources(dir, L2OFF, regions)
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

// TestCacheMatchesHeapLoader pins that a mapped region is byte for byte the
// region the heap loader decodes from the same file.
func TestCacheMatchesHeapLoader(t *testing.T) {
	dir := t.TempDir()
	regions := []Region{{X: 20, Y: 20}, {X: 21, Y: 20}}
	for _, r := range regions {
		writeMixedL2OFF(t, dir, r.X, r.Y, -50)
	}
	cachePath := filepath.Join(t.TempDir(), "geodata.cache")
	if _, err := WriteCache(cachePath, dir, L2OFF, regions, zerolog.Nop()); err != nil {
		t.Fatalf("WriteCache(): %v", err)
	}
	mapped, err := openCache(cachePath, L2OFF, mustStat(t, dir, regions))
	if err != nil {
		t.Fatalf("openCache(): %v", err)
	}
	for i, r := range regions {
		heap, _, err := readRegion(regionPath(dir, L2OFF, r.X, r.Y), L2OFF)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(storedBytes(t, mapped[i]), storedBytes(t, heap)) {
			t.Fatalf("region %v: mapped bytes differ from the heap loader's", r)
		}
	}
}

// TestWriteCacheReplacesAtomically pins that rebuilding writes a new file
// and renames it into place: regions mapped from the old cache keep their
// answers, and no temporary file is left behind.
func TestWriteCacheReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	regions := []Region{{X: 20, Y: 20}}
	writeMixedL2OFF(t, dir, 20, 20, 64)
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "geodata.cache")
	if _, err := WriteCache(cachePath, dir, L2OFF, regions, zerolog.Nop()); err != nil {
		t.Fatal(err)
	}
	old, err := openCache(cachePath, L2OFF, mustStat(t, dir, regions))
	if err != nil {
		t.Fatal(err)
	}

	writeMixedL2OFF(t, dir, 20, 20, 72)
	if _, err := WriteCache(cachePath, dir, L2OFF, regions, zerolog.Nop()); err != nil {
		t.Fatal(err)
	}
	if got := old[0].HeightNearest(0, 0, 0, 0, 0); got != 64 {
		t.Fatalf("old mapping height = %d, want 64", got)
	}
	current, err := openCache(cachePath, L2OFF, mustStat(t, dir, regions))
	if err != nil {
		t.Fatal(err)
	}
	if got := current[0].HeightNearest(0, 0, 0, 0, 0); got != 72 {
		t.Fatalf("new mapping height = %d, want 72", got)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("cache dir holds %d files, want only the cache", len(entries))
	}
}
