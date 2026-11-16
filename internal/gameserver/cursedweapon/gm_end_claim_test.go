package cursedweapon

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// A game master's end of a weapon whose ground item a pickup holds leaves
// that item dead: the pickup arms nothing with it, and one that lets go
// takes it out of the world (#5257, follow-up of the owner-approved #5149
// fix).

// droppedHeld brings a weapon out on the ground as ground and has a pickup
// claim it.
func droppedHeld(t *testing.T, m *Manager) (int32, *fakeGround) {
	t.Helper()
	id, ok := m.RollDrop(always(0))
	if !ok {
		t.Fatal("no drop")
	}
	ground := &fakeGround{id: 4242}
	m.PlaceOnGround(id, ground, location.Location{X: 1, Y: 2, Z: 3})
	if !ground.Claim() {
		t.Fatal("the pickup could not claim the fresh drop")
	}
	return id, ground
}

// TestGMEndLeavesAHeldPickupDead: //cw remove ends the weapon a pickup
// holds and leaves the ground item to the pickup, which then arms nothing:
// the weapon stays not out.
func TestGMEndLeavesAHeldPickupDead(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	m, _ := newManager(t, &fakeStore{}, now)
	id, ground := droppedHeld(t, m)

	end, ok := m.End(id)
	if !ok || end.GroundObjectID != 0 || end.Held {
		t.Fatalf("End = %+v %v, want an end that leaves the held ground item alone", end, ok)
	}
	res, ok := m.Activate(id, Holder{ObjectID: 7, Karma: 100, PKKills: 3, ItemObjectID: ground.id}, always(0))
	if !ok || !res.Dead {
		t.Fatalf("Activate of the ended weapon's item = %+v %v, want it dead", res, ok)
	}
	if active := m.Active(); len(active) != 0 {
		t.Fatalf("Active = %+v, want none", active)
	}
	// A later drop of the weapon arms its picker as usual.
	again, _ := droppedHeld(t, m)
	if res, ok := m.Activate(again, Holder{ObjectID: 7, ItemObjectID: ground.id}, always(0)); !ok || res.Dead {
		t.Fatalf("Activate of the next drop = %+v %v, want it armed", res, ok)
	}
}

// TestGMEndClaimsAFreeGroundItem: the end of a weapon nothing holds takes
// its ground item's claim for the removal.
func TestGMEndClaimsAFreeGroundItem(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	m, _ := newManager(t, &fakeStore{}, now)
	id, ground := droppedHeld(t, m)
	ground.Release()

	end, ok := m.End(id)
	if !ok || end.GroundObjectID != ground.id || !end.GroundClaimed || !ground.claimed {
		t.Fatalf("End = %+v %v (claimed %v), want the ground item claimed for removal", end, ok, ground.claimed)
	}
}

// TestGMReloadLeavesAHeldPickupDead: //reload cw ends the weapon and takes
// fresh ones; the held pickup arms none of them, and a holder of another
// weapon takes no stage from it.
func TestGMReloadLeavesAHeldPickupDead(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	m, _ := newManager(t, &fakeStore{}, now)
	id, ground := droppedHeld(t, m)

	for _, end := range m.Reload(testTable(t)) {
		if end.GroundObjectID != 0 {
			t.Fatalf("Reload end %+v takes the held ground item", end)
		}
	}
	res, ok := m.Activate(id, Holder{ObjectID: 7, HeldItemID: zariche, ItemObjectID: ground.id}, always(0))
	if !ok || !res.Dead || res.RankedUp || res.Assimilated {
		t.Fatalf("Activate after the reload = %+v %v, want it dead", res, ok)
	}
	if active := m.Active(); len(active) != 0 {
		t.Fatalf("Active = %+v, want none", active)
	}
}

// TestLetGoOfADeadItem: a pickup that took nothing lets go of a live
// weapon's item, but keeps the claim on a dead one for its removal.
func TestLetGoOfADeadItem(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	m, _ := newManager(t, &fakeStore{}, now)
	id, ground := droppedHeld(t, m)
	if m.LetGo(ground) || ground.claimed {
		t.Fatal("LetGo of a live weapon's item kept the claim")
	}
	ground.Claim()
	m.End(id)
	if !m.LetGo(ground) || !ground.claimed {
		t.Fatal("LetGo of the ended weapon's item let it go")
	}
	if res, _ := m.Activate(id, Holder{ObjectID: 7, ItemObjectID: ground.id}, always(0)); res.Dead {
		t.Fatal("the dead item stayed dead after its removal")
	}
}
