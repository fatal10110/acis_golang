package script

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// objectIDOf returns the object id of n, 0 for nil.
func objectIDOf(n *NPC) int32 {
	if n == nil {
		return 0
	}
	return n.ObjectID()
}

// An NPC map cell keys by NPC identity, as the reference's map keyed by an
// NPC object does: get, contains, put, remove, and the removal of a
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

// Two lives of one spawn slot are one NPC to the cell, as the reference
// respawns the same NPC object: a key set on the first life is held for the
// second, a value set as the first life is removed as the second, and Get
// hands back the life last set. Another slot's NPC of the same template
// stays another key.
func TestNPCMapCellSpawnSlotLives(t *testing.T) {
	var c NPCMapCell
	slot, other := npc.NewScratch(), npc.NewScratch()
	life := func(objectID int32, s *npc.Scratch) *NPC {
		return &NPC{self: &timerNPCFake{combatant: combatant{objectID: objectID}, id: 27286, scratch: s}}
	}
	first, second, elsewhere := life(100, slot), life(101, slot), life(102, other)
	tablet := newNPCHandle(t, 200, 31650)

	c.Set(first, tablet)
	if !c.Has(second) || objectIDOf(c.Get(second)) != 200 || !c.Has(first) {
		t.Fatal("the slot's second life is not the key its first life set")
	}
	if c.Has(elsewhere) {
		t.Fatal("another slot's NPC of the same template is held")
	}
	c.Set(second, tablet)
	if len(c.entries) != 1 {
		t.Fatalf("second life added an entry: %d entries", len(c.entries))
	}

	c.Set(tablet, first)
	c.Set(tablet, second)
	c.mu.Lock()
	held := c.entries[c.index(npcIdentity(tablet))].value
	c.mu.Unlock()
	if held != second.self {
		t.Fatalf("tablet holds %v, want the life last set", held)
	}
	c.Set(elsewhere, first)
	if c.DeleteValue(life(103, other)) {
		t.Fatal("removed a value of another slot nothing maps to")
	}
	// The tablet maps to the second life and elsewhere to the first: removing
	// the first life as a value drops the tablet's entry, set earlier.
	if !c.DeleteValue(first) || c.Has(tablet) || !c.Has(elsewhere) {
		t.Fatal("DeleteValue(first life) did not drop the tablet's entry only")
	}
	if !c.DeleteValue(second) || c.Has(elsewhere) {
		t.Fatal("DeleteValue(second life) did not drop elsewhere's entry")
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
