package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// crowdSightNPC places n NPCs around the seer, all in its surrounding
// regions: more than a 32-entry known-list buffer holds.
func crowdSightNPC(w *world.State, n int) {
	for i := range n {
		x := sightX - 1500 + (i%10)*300
		y := sightY - 1500 + (i/10)*300
		w.Spawn(&sightCreature{id: int32(100 + i), kind: actor.KindNPC}, x, y, 0, 0)
	}
}

// TestScanSightAllocatesNothing pins that an awake NPC's tick scan builds
// no heap snapshot of its surroundings: none when only NPCs stand around
// it, and none with a playable it has already been told about among them.
func TestScanSightAllocatesNothing(t *testing.T) {
	t.Run("npcs only", func(t *testing.T) {
		w, n := newSightNPC(false, false)
		crowdSightNPC(w, 40)
		if allocs := testing.AllocsPerRun(50, func() { scanSight(n) }); allocs != 0 {
			t.Fatalf("scan allocs = %v per run, want 0", allocs)
		}
		if len(n.told) != 0 {
			t.Fatalf("calls = %v, want none for NPCs", n.told)
		}
	})
	t.Run("one seen player", func(t *testing.T) {
		w, n := newSightNPC(false, false)
		crowdSightNPC(w, 40)
		p := &sightCreature{id: 2, kind: actor.KindPlayer}
		w.Spawn(p, sightX+100, sightY, 0, 0)
		scanSight(n)
		if len(n.told) != 1 {
			t.Fatalf("calls = %v, want the player once", n.told)
		}
		if allocs := testing.AllocsPerRun(50, func() { scanSight(n) }); allocs != 0 {
			t.Fatalf("scan allocs = %v per run, want 0", allocs)
		}
		if len(n.told) != 1 {
			t.Fatalf("calls = %v, want the player told once", n.told)
		}
	})
}

// TestScanSightTellsSummonInQuietRegion pins that a summon whose owner
// stands beyond the NPC's surrounding regions is still told about: no
// player keeps the region active, yet the summon is a known playable.
func TestScanSightTellsSummonInQuietRegion(t *testing.T) {
	w, n := newSightNPC(false, false)
	owner := &sightCreature{id: 2, kind: actor.KindPlayer}
	w.Spawn(owner, sightX+20000, sightY, 0, 0)
	pet := &sightCreature{id: 3, kind: actor.KindSummon, owner: owner}
	w.Spawn(pet, sightX+100, sightY, 0, 0)
	if w.RegionActive(n) {
		t.Fatal("the NPC's region is active, want it quiet with the owner far away")
	}
	scanSight(n)
	if len(n.told) != 1 || n.told[0] != pet.id {
		t.Fatalf("calls = %v, want the summon %d", n.told, pet.id)
	}
}

// BenchmarkScanSight measures an awake NPC's tick scan among 40 NPCs, with
// no playable and with one already-seen player.
func BenchmarkScanSight(b *testing.B) {
	b.Run("npcs", func(b *testing.B) {
		w, n := newSightNPC(false, false)
		crowdSightNPC(w, 40)
		b.ReportAllocs()
		for b.Loop() {
			scanSight(n)
		}
	})
	b.Run("npcs+player", func(b *testing.B) {
		w, n := newSightNPC(false, false)
		crowdSightNPC(w, 40)
		w.Spawn(&sightCreature{id: 2, kind: actor.KindPlayer}, sightX+100, sightY, 0, 0)
		scanSight(n)
		b.ReportAllocs()
		for b.Loop() {
			scanSight(n)
		}
	})
}
