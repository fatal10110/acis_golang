package cursedweapon

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// A game master's end or reload between a drop and its placement leaves
// that drop stale: PlaceOnGround refuses it and leaves the item claimed for
// the caller to take out of the world, even once the weapon dropped again
// (#5270, follow-up of the owner-approved #5149/#5257 fixes).

func TestPlaceOnGroundRefusesADropEndedBeforeIt(t *testing.T) {
	t.Parallel()
	at := location.Location{X: 1, Y: 2, Z: 3}
	for _, tc := range []struct {
		name string
		end  func(m *Manager, itemID int32)
	}{
		{name: "end", end: func(m *Manager, itemID int32) { m.End(itemID) }},
		{name: "reload", end: func(m *Manager, _ int32) { m.Reload(testTable(t)) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := newManager(t, &fakeStore{}, time.Unix(1000, 0))
			stale, ok := m.RollDrop(always(0))
			if !ok {
				t.Fatal("no drop")
			}
			tc.end(m, stale.ItemID)
			ground := &fakeGround{id: 4242, claimed: true}
			if m.PlaceOnGround(stale, ground, at) {
				t.Fatal("PlaceOnGround took a drop ended before it")
			}
			if !ground.claimed {
				t.Fatal("the refused drop's item lost its claim")
			}
			if active := m.Active(); len(active) != 0 {
				t.Fatalf("weapons out = %+v, want none", active)
			}

			// The weapon drops again: the stale drop still finds nothing,
			// and the new one is placed and let go.
			again, ok := m.RollDrop(always(0))
			if !ok || again.ItemID != stale.ItemID {
				t.Fatalf("second drop = %d %v, want %d", again.ItemID, ok, stale.ItemID)
			}
			if m.PlaceOnGround(stale, &fakeGround{id: 4243, claimed: true}, at) {
				t.Fatal("PlaceOnGround took the stale drop over the new one")
			}
			fresh := &fakeGround{id: 4244, claimed: true}
			if !m.PlaceOnGround(again, fresh, at) {
				t.Fatal("PlaceOnGround refused the new drop")
			}
			if fresh.claimed {
				t.Fatal("the placed item kept its claim")
			}
			end, ok := m.End(again.ItemID)
			if !ok || end.GroundObjectID != fresh.id || !end.GroundClaimed {
				t.Fatalf("end = %+v %v, want the placed item %d claimed", end, ok, fresh.id)
			}
		})
	}
}
