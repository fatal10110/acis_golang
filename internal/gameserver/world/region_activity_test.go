package world

import (
	"sync"
	"testing"
)

// playerStub is a grid occupant that counts toward Region activity, like a
// live player character.
type playerStub struct {
	trackedStub
}

func (p *playerStub) WorldPlayer() {}

func TestRegionActivatesOnPlayerSpawnAndDeactivatesOnDespawn(t *testing.T) {
	s := New()

	p := &playerStub{trackedStub: trackedStub{id: 1}}
	s.Spawn(p, 0, 0, 0, 0)

	region, ok := s.RegionAt(0, 0)
	if !ok {
		t.Fatal("RegionAt(0, 0) not found")
	}
	if !region.Active() {
		t.Fatal("region did not activate on player spawn")
	}
	if !s.RegionActive(p) {
		t.Fatal("RegionActive(p) = false right after spawn, want true")
	}

	s.Despawn(p)

	if region.Active() {
		t.Fatal("region stayed active after its only player despawned")
	}
	if s.RegionActive(p) {
		t.Fatal("RegionActive(p) = true after despawn, want false (off the grid)")
	}
}

// TestRegionStaysActiveWhileAnyNeighborHasAPlayer covers the neighborhood
// (not per-region) nature of activation: a region deactivates only once
// none of the 3x3 regions around it holds a player, even if the player
// that most recently occupied it specifically has left.
func TestRegionStaysActiveWhileAnyNeighborHasAPlayer(t *testing.T) {
	s := New()

	a := &playerStub{trackedStub: trackedStub{id: 1}}
	s.Spawn(a, 0, 0, 0, 0)
	b := &playerStub{trackedStub: trackedStub{id: 2}}
	s.Spawn(b, regionSize, 0, 0, 0) // adjacent region, within a's neighborhood

	regionA, _ := s.RegionAt(0, 0)
	regionB, _ := s.RegionAt(regionSize, 0)
	if !regionA.Active() || !regionB.Active() {
		t.Fatal("adjacent regions did not both activate")
	}

	s.Despawn(a)

	if !regionA.Active() {
		t.Fatal("region deactivated even though a neighboring region still has a player")
	}
	if !regionB.Active() {
		t.Fatal("region with the remaining player deactivated")
	}

	s.Despawn(b)

	if regionA.Active() || regionB.Active() {
		t.Fatal("regions stayed active after every nearby player left")
	}
}

func TestNonPlayerPresenceDoesNotActivateRegion(t *testing.T) {
	s := New()

	npc := &trackedStub{id: 1}
	s.Spawn(npc, 0, 0, 0, 0)

	region, _ := s.RegionAt(0, 0)
	if region.Active() {
		t.Fatal("region activated from a non-player object alone")
	}
	if s.RegionActive(npc) {
		t.Fatal("RegionActive(npc) = true with no player nearby")
	}
}

// TestRegionActiveGatesSchedulerShapedWork demonstrates the intended
// consumer: a per-object scheduler (AI/follow/route walking) skips objects
// sitting in a region with no player nearby.
func TestRegionActiveGatesSchedulerShapedWork(t *testing.T) {
	s := New()

	npcNearPlayer := &trackedStub{id: 1}
	s.Spawn(npcNearPlayer, 0, 0, 0, 0)
	npcFarFromPlayer := &trackedStub{id: 2}
	s.Spawn(npcFarFromPlayer, 8192, 0, 0, 0)
	player := &playerStub{trackedStub: trackedStub{id: 3}}
	s.Spawn(player, 0, 0, 0, 0)

	ticked := map[int32]bool{}
	for _, npc := range []*trackedStub{npcNearPlayer, npcFarFromPlayer} {
		if !s.RegionActive(npc) {
			continue
		}
		ticked[npc.id] = true
	}

	if !ticked[npcNearPlayer.id] {
		t.Fatal("scheduler skipped an npc sharing an active region with a player")
	}
	if ticked[npcFarFromPlayer.id] {
		t.Fatal("scheduler ticked an npc in a region with no nearby player")
	}
}

// TestRegionActivityNotifiesNonPlayerMovingIntoAlreadyInactiveRegion covers
// the gap issue #816 filed against #812/#815: a non-player crossing from an
// active region straight into a neighbor that was already inactive doesn't
// flip that neighbor's active flag (only a Player's presence does), so no
// setActive transition fires there. Region.Add's own current-state notify
// is what tells the arriving object it landed in dead space — without it,
// nothing would, until (if ever) some other event happened to re-run its
// Tick/Think.
func TestRegionActivityNotifiesNonPlayerMovingIntoAlreadyInactiveRegion(t *testing.T) {
	s := New()

	player := &playerStub{trackedStub: trackedStub{id: 1}}
	s.Spawn(player, 0, 0, 0, 0)

	obj := &activeTrackedStub{trackedStub: trackedStub{id: 2}}
	s.Spawn(obj, 0, 0, 0, 0)
	obj.activeCalls = 0
	obj.inactiveCalls = 0

	const far = 8192 // several regions away: outside player's 3x3 neighborhood
	if err := s.Move(obj, far, 0, 0); err != nil {
		t.Fatalf("Move: %v", err)
	}

	dest, ok := s.RegionAt(far, 0)
	if !ok {
		t.Fatal("RegionAt(far, 0) not found")
	}
	if dest.Active() {
		t.Fatal("destination region activated by a non-player move, want inactive")
	}
	if obj.inactiveCalls != 1 {
		t.Fatalf("inactive calls after moving into an already-inactive region = %d, want 1 (no Tick required)", obj.inactiveCalls)
	}
}

// TestRegionActivityConcurrentPlayerChurn guards against a check-then-act
// race: one player's departure could read a stale (pre-arrival) player
// count for a region and deactivate it right after another player's
// concurrent arrival had just activated it. regionActivityMu serializes
// player relocations against each other, so a permanently present player
// must leave the region active no matter how much unrelated player traffic
// churns through concurrently.
func TestRegionActivityConcurrentPlayerChurn(t *testing.T) {
	s := New()

	const churners = 32
	const cycles = 200

	var wg sync.WaitGroup
	wg.Add(1 + churners)

	go func() {
		defer wg.Done()
		s.Spawn(&playerStub{trackedStub: trackedStub{id: 1}}, 0, 0, 0, 0)
	}()
	for i := 0; i < churners; i++ {
		go func(id int32) {
			defer wg.Done()
			p := &playerStub{trackedStub: trackedStub{id: id}}
			for j := 0; j < cycles; j++ {
				s.Spawn(p, 0, 0, 0, 0)
				s.Despawn(p)
			}
		}(int32(100 + i))
	}
	wg.Wait()

	region, ok := s.RegionAt(0, 0)
	if !ok {
		t.Fatal("RegionAt(0, 0) not found")
	}
	if !region.Active() {
		t.Fatal("region with a permanently present player went inactive under concurrent player churn")
	}
}
