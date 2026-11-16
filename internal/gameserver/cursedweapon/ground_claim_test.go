package cursedweapon

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// fakeGround is a weapon's ground item: one claim at a time.
type fakeGround struct {
	id      int32
	claimed bool
}

func (g *fakeGround) ObjectID() int32 { return g.id }

func (g *fakeGround) Claim() bool {
	if g.claimed {
		return false
	}
	g.claimed = true
	return true
}

func (g *fakeGround) Release() { g.claimed = false }

// TestGroundEndWaitsForAPickupHoldingTheItem: a pickup holding the ground
// item as the weapon's hour on the ground runs out keeps the weapon out,
// and finishing it makes the picker the holder with the life the drop
// started. The timer never ends a weapon it cannot take off the ground, so
// no picker is armed with a weapon already ended (#5149, owner-approved
// fix).
func TestGroundEndWaitsForAPickupHoldingTheItem(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	m, c := newManager(t, &fakeStore{}, now)
	d, ok := m.RollDrop(always(0))
	if !ok {
		t.Fatal("no drop")
	}
	id := d.ItemID
	ground := &fakeGround{id: 4242}
	if !m.PlaceOnGround(d, ground, location.Location{X: 1, Y: 2, Z: 3}) {
		t.Fatal("PlaceOnGround refused the drop")
	}

	if !ground.Claim() {
		t.Fatal("the pickup could not claim the fresh drop")
	}
	c.now = now.Add(time.Hour)
	if ends, _ := m.Tick(c.now); len(ends) != 0 {
		t.Fatalf("ends with the ground item held = %+v, want none", ends)
	}
	if active := m.Active(); len(active) != 1 || active[0].ItemID != id || active[0].Activated {
		t.Fatalf("Active = %+v, want the weapon still on the ground", active)
	}
	if _, ok := m.Activate(id, Holder{ObjectID: 7, Karma: 100, PKKills: 3}, always(0)); !ok {
		t.Fatal("Activate refused the picked-up weapon")
	}
	if left := m.TimeLeft(id); left != 71*time.Hour {
		t.Fatalf("time left = %v, want the 71h left of the life its drop started", left)
	}
	if ends, _ := m.Tick(now.Add(2 * time.Hour)); len(ends) != 0 {
		t.Fatalf("ends after the pickup = %+v, want none", ends)
	}
	if active := m.Active(); len(active) != 1 || !active[0].Activated || active[0].HolderID != 7 {
		t.Fatalf("Active = %+v, want the weapon held by 7", active)
	}
}

// TestGroundEndRetriesOnceThePickupLetsGo: a pickup that held the ground
// item past the weapon's hour on the ground and then failed lets it go, and
// the next tick ends the weapon, its end holding the item's claim. The same
// holds for a dropped weapon whose life runs out while a pickup holds it.
func TestGroundEndRetriesOnceThePickupLetsGo(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	m, c := newManager(t, &fakeStore{}, now)
	d, ok := m.RollDrop(always(0))
	if !ok {
		t.Fatal("no drop")
	}
	id := d.ItemID
	ground := &fakeGround{id: 4242}
	if !m.PlaceOnGround(d, ground, location.Location{X: 1, Y: 2, Z: 3}) {
		t.Fatal("PlaceOnGround refused the drop")
	}
	ground.Claim()
	if ends, _ := m.Tick(now.Add(time.Hour)); len(ends) != 0 {
		t.Fatalf("ends with the ground item held = %+v, want none", ends)
	}
	ground.claimed = false
	ends, _ := m.Tick(now.Add(time.Hour + time.Second))
	if want := []EndOfLife{{ItemID: id, GroundObjectID: 4242, GroundClaimed: true}}; !slices.Equal(ends, want) || !ground.claimed {
		t.Fatalf("ends once let go = %+v (claimed %v), want %+v holding the claim", ends, ground.claimed, want)
	}

	// The weapon's life runs out while it lies where its holder died.
	if d, ok = m.RollDrop(always(0)); !ok || d.ItemID != id {
		t.Fatal("no second drop")
	}
	if _, ok := m.Activate(id, Holder{ObjectID: 7}, always(0)); !ok {
		t.Fatal("Activate refused")
	}
	c.now = now.Add(2 * time.Hour)
	death, ok := m.HolderDied(id, always(99))
	if !ok || death.Disappeared {
		t.Fatalf("death = %+v %v, want the weapon dropped", death, ok)
	}
	dropped := &fakeGround{id: 4343}
	if !m.PlaceOnGround(death.Drop, dropped, location.Location{X: 1, Y: 2, Z: 3}) {
		t.Fatal("PlaceOnGround refused the holder's drop")
	}
	dropped.Claim()
	if ends, _ := m.Tick(now.Add(73 * time.Hour)); len(ends) != 0 {
		t.Fatalf("ends past the life with the ground item held = %+v, want none", ends)
	}
	dropped.claimed = false
	ends, _ = m.Tick(now.Add(73 * time.Hour))
	if want := []EndOfLife{{ItemID: id, GroundObjectID: 4343, GroundClaimed: true}}; !slices.Equal(ends, want) {
		t.Fatalf("ends once let go = %+v, want %+v", ends, want)
	}
	if active := m.Active(); len(active) != 0 {
		t.Fatalf("Active = %+v, want none", active)
	}
}
