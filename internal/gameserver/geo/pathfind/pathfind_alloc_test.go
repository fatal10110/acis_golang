//go:build !race

package pathfind

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFindIntoReusesSearchScratch(t *testing.T) {
	finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})), DefaultOptions())
	dst := make([]location.Location, 0, 8)

	path, _, ok := finder.FindInto(dst[:0], at(0, 0, 0), at(3, 0, 0))
	if !ok {
		t.Fatal("FindInto() = no path, want path")
	}
	if len(path) == 0 {
		t.Fatal("FindInto() returned empty path, want target point")
	}
	if &path[0] != &dst[:1][0] {
		t.Fatal("FindInto() did not reuse caller path buffer")
	}

	allocs := testing.AllocsPerRun(100, func() {
		path, _, ok := finder.FindInto(dst[:0], at(0, 0, 0), at(3, 0, 0))
		if !ok || len(path) == 0 || path[len(path)-1] != at(3, 0, 0) {
			panic("FindInto() returned wrong path")
		}
	})
	if allocs != 0 {
		t.Fatalf("FindInto() allocations = %v, want 0", allocs)
	}
}

func TestHasPathReusesSearchScratch(t *testing.T) {
	finder := New(newTestEngine(t, complexBlock(func(x, y int) block.Cell {
		return block.Cell{Height: 0, NSWE: block.AllDirections}
	})), DefaultOptions())

	if !finder.HasPath(at(0, 0, 0), at(3, 0, 0)) {
		t.Fatal("HasPath() = false, want true")
	}

	allocs := testing.AllocsPerRun(100, func() {
		if !finder.HasPath(at(0, 0, 0), at(3, 0, 0)) {
			panic("HasPath() = false")
		}
	})
	if allocs != 0 {
		t.Fatalf("HasPath() allocations = %v, want 0", allocs)
	}
}
