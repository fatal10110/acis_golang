package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// dropAkamanahAt brings Akamanah out as a monster's drop does, lying at
// the feet of id: its life and its hour on the ground start at the
// weapons' clock.
func dropAkamanahAt(t *testing.T, srv *gameservertest.Server, c *scriptedClient, id int32) *grounditem.Item {
	t.Helper()
	drop, ok := srv.CursedWeapons.RollDrop(func(int) int { return 0 })
	if !ok || drop.ItemID != akamanahID {
		t.Fatalf("drop = %d %v, want Akamanah", drop.ItemID, ok)
	}
	x, y, z := srv.PlayerPosition(t, id)
	srv.SeedGroundItem(t, 0, akamanahID, 1, x, y, z)
	drainUntilQuiet(t, c)
	ground := groundWeapon(t, srv, akamanahID)
	if !srv.CursedWeapons.PlaceOnGround(drop, ground, location.Location{X: x, Y: y, Z: z}) {
		t.Fatal("PlaceOnGround refused the drop")
	}
	return ground
}

// TestCursedWeaponGroundTimerWaitsForAHeldPickup: Akamanah's hour on the
// ground runs out while a pickup holds its ground item. The timer cannot
// take the item off the ground, so it leaves the weapon out and tells no
// one it disappeared; the pickup then makes its picker the holder, with
// the life the drop started. Ending the weapon there would let the pickup
// arm a weapon already announced gone and with no life left to run out
// (#5149, owner-approved fix). The test holds the claim the way the
// pickup's click does, then lets the click take it.
func TestCursedWeaponGroundTimerWaitsForAHeldPickup(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	clock := newCursedClock(base)
	srv := cursedBoot(t, 10,
		gameservertest.WithSeed(seedDropVictim(25, 100, 3)),
		gameservertest.WithCursedWeaponClock(clock.Now),
	)
	c, id := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	setVictimRoll(t, srv, id, 0)
	ground := dropAkamanahAt(t, srv, c, id)

	if !ground.Claim() {
		t.Fatal("the fresh drop refused its first claim")
	}
	clock.now.Store(base.Add(time.Hour).UnixNano())
	srv.TickCursedWeapons(clock.Now())
	srv.Settle(t)
	if gone := indexOfMessage(readQuiet(c), 0, serverpackets.SystemMessageS1HasDisappeared, itemParam(akamanahID)); gone >= 0 {
		t.Fatal("the ground timer announced a weapon a pickup held")
	}
	if active := srv.CursedWeapons.Active(); len(active) != 1 || active[0].ItemID != akamanahID || active[0].Activated {
		t.Fatalf("weapons out = %+v, want Akamanah still on the ground", active)
	}
	ground.Release()

	x, y, z := srv.PlayerPosition(t, id)
	c.Send(encodeAction(ground.ObjectID(), int32(x), int32(y), int32(z), false))
	srv.AdvanceUntil(t, "weapon picked up", func() bool {
		_, lying := srv.State.Object(ground.ObjectID())
		return !lying
	})
	srv.SettlePickup(t, id)
	frames := readQuiet(c)
	picked := indexOfMessage(frames, 0, serverpackets.SystemMessageYouPickedUpS1, itemParam(akamanahID))
	appeared := indexOfMessage(frames, picked, serverpackets.SystemMessageOwnerOfS2AppearedInS1Region, zoneParam(x, y, z), itemParam(akamanahID))
	if picked < 0 || appeared < 0 {
		t.Fatalf("picked up at %d, owner appeared at %d: want both, in that order", picked, appeared)
	}
	if gone := indexOfMessage(frames, 0, serverpackets.SystemMessageS1HasDisappeared, itemParam(akamanahID)); gone >= 0 {
		t.Fatal("the picked-up weapon was announced gone")
	}
	holder := victimCharacter(t, srv, id)
	if holder.CursedWeaponID() != akamanahID || holder.Karma() != 9999999 {
		t.Fatalf("holder = weapon %d karma %d, want Akamanah, 9999999", holder.CursedWeaponID(), holder.Karma())
	}
	if left := srv.CursedWeapons.TimeLeft(akamanahID); left != 71*time.Hour {
		t.Fatalf("Akamanah time left = %v, want the 71h left of the life its drop started", left)
	}
}

// TestCursedWeaponGroundTimerEndsOnceThePickupLetsGo: a pickup that held
// Akamanah's ground item past its hour on the ground and then failed lets
// the item go, and the timer's next tick ends the weapon: the item leaves
// the world and everyone hears it disappeared.
func TestCursedWeaponGroundTimerEndsOnceThePickupLetsGo(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	clock := newCursedClock(base)
	srv := cursedBoot(t, 10,
		gameservertest.WithSeed(seedDropVictim(25, 100, 3)),
		gameservertest.WithCursedWeaponClock(clock.Now),
	)
	c, id := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	ground := dropAkamanahAt(t, srv, c, id)

	ground.Claim()
	clock.now.Store(base.Add(time.Hour).UnixNano())
	srv.TickCursedWeapons(clock.Now())
	srv.Settle(t)
	readQuiet(c)
	ground.Release()

	clock.now.Store(base.Add(time.Hour + time.Second).UnixNano())
	srv.TickCursedWeapons(clock.Now())
	srv.Settle(t)
	frames := readQuiet(c)
	deleted := indexOf(frames, 0, serverpackets.OpcodeDeleteObject, ground.ObjectID())
	gone := indexOfMessage(frames, 0, serverpackets.SystemMessageS1HasDisappeared, itemParam(akamanahID))
	if deleted < 0 || gone < 0 {
		t.Fatalf("DeleteObject of the weapon at %d, S1_HAS_DISAPPEARED at %d: want both", deleted, gone)
	}
	if _, lying := srv.State.Object(ground.ObjectID()); lying {
		t.Fatal("the ended weapon still lies on the ground")
	}
	if active := srv.CursedWeapons.Active(); len(active) != 0 {
		t.Fatalf("weapons out = %+v, want none", active)
	}
	if ground.Claim() {
		t.Fatal("the removed item accepted another claim")
	}
}
