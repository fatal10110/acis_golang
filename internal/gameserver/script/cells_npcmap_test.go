package script

import (
	"sync"
	"testing"
)

// objectIDOf returns the object id of n, 0 for nil.
func objectIDOf(n *NPC) int32 {
	if n == nil {
		return 0
	}
	return n.ObjectID()
}

// An NPC map cell keys by the NPC itself, as the reference's map keyed by
// an NPC object does: get, contains, put, remove, and the removal of a
// value from the map's values, which drops one entry mapping to it.
func TestNPCMapCellByIdentity(t *testing.T) {
	var c NPCMapCell
	tablet := newNPCHandle(t, 100, 31650)
	twin := newNPCHandle(t, 101, 31650)
	corrupted := newNPCHandle(t, 200, 27286)
	defender := newNPCHandle(t, 300, 31631)

	if c.Has(tablet) || c.Get(tablet) != nil {
		t.Fatal("zero cell holds the tablet")
	}
	c.Set(tablet, corrupted)
	c.Set(nil, corrupted)
	c.Set(twin, nil)
	c.Set(NPCOf(nil), corrupted)
	if got := objectIDOf(c.Get(NPCOf(tablet.self))); got != 200 {
		t.Fatalf("Get(tablet) through a fresh handle = %d, want 200", got)
	}
	if c.Has(twin) || c.Get(twin) != nil || c.Has(nil) || c.Get(nil) != nil {
		t.Fatal("another NPC of the tablet's template, or nil, is held")
	}
	c.Set(tablet, defender)
	if got := objectIDOf(c.Get(tablet)); got != 300 {
		t.Fatalf("Get(tablet) after a second Set = %d, want 300", got)
	}

	// Two keys map to the defender: removing it as a value drops the
	// first key set, then the second.
	c.Set(twin, defender)
	if c.DeleteValue(corrupted) || c.DeleteValue(nil) {
		t.Fatal("removed a value nothing maps to")
	}
	if !c.DeleteValue(defender) || c.Has(tablet) || !c.Has(twin) {
		t.Fatal("DeleteValue did not drop the first entry only")
	}
	if !c.DeleteValue(defender) || c.Has(twin) {
		t.Fatal("DeleteValue did not drop the second entry")
	}

	c.Set(defender, corrupted)
	if c.Delete(tablet) || c.Delete(nil) {
		t.Fatal("Delete of a key not held reported true")
	}
	if !c.Delete(defender) || c.Has(defender) {
		t.Fatal("Delete kept the key")
	}
}

// Concurrent writers and readers keep each key's last value and lose no
// key.
func TestNPCMapCellConcurrent(t *testing.T) {
	var c NPCMapCell
	keys := make([]*NPC, 8)
	values := make([]*NPC, 8)
	for i := range keys {
		keys[i] = newNPCHandle(t, int32(400+i), 31650)
		values[i] = newNPCHandle(t, int32(500+i), 27286)
	}
	var wg sync.WaitGroup
	for i := range keys {
		wg.Go(func() {
			for range 100 {
				c.Set(keys[i], values[i])
				c.Get(keys[(i+1)%len(keys)])
				c.DeleteValue(values[(i+2)%len(values)])
				c.Set(keys[i], values[i])
			}
		})
	}
	wg.Wait()
	for i, k := range keys {
		if got := objectIDOf(c.Get(k)); got != 0 && got != values[i].ObjectID() {
			t.Fatalf("key %d maps to %d", i, got)
		}
	}
}
