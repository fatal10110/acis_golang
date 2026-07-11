package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestIndexAttachAndLookup(t *testing.T) {
	ix := NewIndex()
	// A zone comfortably inside one region.
	small := NewPeace(1, NewCuboid(100, 200, 100, 200, -100, 100))
	// A zone crossing a region boundary (regions are 2048 wide, so this
	// spans the x boundary at 2048).
	wide := NewScript(2, NewCuboid(2000, 2100, 100, 200, -100, 100))
	ix.Add(small)
	ix.Add(wide)

	if got := len(ix.All()); got != 2 {
		t.Fatalf("All() = %d zones, want 2", got)
	}
	if _, ok := ix.ByID(2); !ok {
		t.Fatal("ByID(2) missed")
	}
	if _, ok := ix.ByID(99); ok {
		t.Fatal("ByID(99) found a ghost")
	}

	if zones := ix.At(150, 150); len(zones) != 2 {
		t.Fatalf("At(150,150) = %d zones, want 2 (both attach to the first region)", len(zones))
	}
	// The boundary-crossing zone must be reachable from both sides.
	if _, ok := FindAt[*Script](ix, 2049, 150, 0); !ok {
		t.Fatal("boundary zone not attached to the region right of the boundary")
	}
	if _, ok := FindAt[*Script](ix, 2040, 150, 0); !ok {
		t.Fatal("boundary zone not attached to the region left of the boundary")
	}
	// Out of world bounds.
	if zones := ix.At(-9999999, 0); zones != nil {
		t.Fatal("At() outside the world returned zones")
	}
}

func TestIndexFindAndKindQueries(t *testing.T) {
	ix := NewIndex()
	peace := NewPeace(1, NewCuboid(0, 1000, 0, 1000, -100, 100))
	script := NewScript(2, NewCuboid(0, 1000, 0, 1000, -100, 100))
	ix.Add(peace)
	ix.Add(script)

	if z, ok := FindAt[*Peace](ix, 500, 500, 0); !ok || z.ID() != 1 {
		t.Fatal("FindAt missed the peace zone")
	}
	if _, ok := FindAt[*Peace](ix, 500, 500, 5000); ok {
		t.Fatal("FindAt matched above the zone's volume")
	}
	if _, ok := FindAtXY[*Script](ix, 500, 500); !ok {
		t.Fatal("FindAtXY missed the script zone")
	}
	if got := len(OfKind[*Peace](ix)); got != 1 {
		t.Fatalf("OfKind[*Peace] = %d, want 1", got)
	}
}

func TestIndexRevalidateAndRemoveFrom(t *testing.T) {
	ix := NewIndex()
	peace := NewPeace(1, NewCuboid(0, 1000, 0, 1000, -100, 100))
	ix.Add(peace)

	a := newFakePlayer(7, location.Location{X: 500, Y: 500, Z: 0})
	ix.Revalidate(a)
	if !peace.Inside(a) || !a.flags.Has(FlagPeace) {
		t.Fatal("index revalidation did not admit the actor")
	}

	// Moving out within the same region: revalidation evicts.
	a.pos = location.Location{X: 1500, Y: 1500, Z: 0}
	ix.Revalidate(a)
	if peace.Inside(a) || a.flags.Has(FlagPeace) {
		t.Fatal("index revalidation did not evict the actor")
	}

	// Region-change eviction path.
	a.pos = location.Location{X: 500, Y: 500, Z: 0}
	ix.Revalidate(a)
	ix.RemoveFrom(a, 500, 500)
	if peace.Inside(a) || a.flags.Has(FlagPeace) {
		t.Fatal("RemoveFrom left the actor tracked")
	}
}
