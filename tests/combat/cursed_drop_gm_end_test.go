package combat

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A game master's end of a cursed weapon that lands between its drop and
// the drop's record of where its item lies never leaves an armable item on
// the ground: the item leaves the world at once, and no region hears of the
// drop (#5270, follow-up of the owner-approved #5149/#5257 fixes).

// endOnDrop ends itemID's weapon, as a game master's //cw remove does, the
// moment its item is laid on the ground: after the drop, before it is
// recorded. It returns the dropped item's object id once seen.
func endOnDrop(t *testing.T, srv *gameservertest.Server, itemID int32) *atomic.Int32 {
	t.Helper()
	var dropped atomic.Int32
	srv.ObserveSends(t, func(payload []byte) {
		if len(payload) < 13 || payload[0] != serverpackets.OpcodeDropItem ||
			int32(binary.LittleEndian.Uint32(payload[9:13])) != itemID {
			return
		}
		if dropped.CompareAndSwap(0, int32(binary.LittleEndian.Uint32(payload[5:9]))) {
			if _, ok := srv.CursedWeapons.End(itemID); !ok {
				t.Errorf("End(%d) found no cursed weapon", itemID)
			}
		}
	})
	return &dropped
}

// assertEndedDropGone checks the weapon dropped as objectID is ended with
// nothing of it left: not on the ground, not out, and no stored row.
func assertEndedDropGone(t *testing.T, srv *gameservertest.Server, frames [][]byte, objectID, itemID int32) {
	t.Helper()
	if objectID == 0 {
		t.Fatal("the weapon never dropped")
	}
	if _, lying := srv.State.Object(objectID); lying {
		t.Fatal("the ended weapon's item stays on the ground")
	}
	for _, obj := range srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == itemID {
			t.Fatalf("item %d lies on the ground as %d", itemID, g.ObjectID())
		}
	}
	deleted := false
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeDeleteObject:
			deleted = deleted || int32(binary.LittleEndian.Uint32(f[1:5])) == objectID
		case serverpackets.OpcodeSystemMessage:
			if id, _ := cursedMessage(f); id == serverpackets.SystemMessageS2WasDroppedInTheS1Region {
				t.Fatal("an ended weapon's drop was announced")
			}
		}
	}
	if !deleted {
		t.Fatal("no DeleteObject for the ended weapon's item")
	}
	if active := srv.CursedWeapons.Active(); len(active) != 0 {
		t.Fatalf("weapons out = %+v, want none", active)
	}
	srv.FlushItems(t)
	ctx := context.Background()
	var items, rows int
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE item_id = ?", itemID).Scan(&items); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM cursed_weapons").Scan(&rows); err != nil {
		t.Fatalf("count cursed_weapons: %v", err)
	}
	if items != 0 || rows != 0 {
		t.Fatalf("stored items %d, cursed weapon rows %d; want none", items, rows)
	}
}

// TestCursedWeaponEndedBeforeMonsterDropPlaced: Akamanah drops from a
// monster kill and a game master ends it before the drop is recorded.
func TestCursedWeaponEndedBeforeMonsterDropPlaced(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10, gameservertest.WithCharacter("Newbie", 5, 0))
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	dropped := endOnDrop(t, srv, akamanahID)

	setVictimRoll(t, srv, objID, 0)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	castKillSkill(t, srv, c, objID, hostile.ObjectID(), false)
	srv.AdvanceUntil(t, "monster death", func() bool { return hostile.CurrentHP() <= 0 })
	srv.Settle(t)

	frames := readQuiet(c)
	assertEndedDropGone(t, srv, frames, dropped.Load(), akamanahID)
	if indexOf(frames, 0, serverpackets.OpcodeEarthquake, -1) >= 0 {
		t.Fatal("an ended weapon's drop shook the ground")
	}
}

// TestCursedWeaponEndedBeforeHolderDropPlaced: Zariche's holder dies and
// drops it, and a game master ends it before the drop is recorded. The
// holder still gets its karma and PK kills back.
func TestCursedWeaponEndedBeforeHolderDropPlaced(t *testing.T) {
	t.Parallel()
	srv := cursedBoot(t, 10,
		gameservertest.WithSeed(seedDropVictim(25, 100, 3)),
		gameservertest.WithDeathDrop(playerDeathDropEverything()),
	)
	c, id := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	pickUpCursed(t, srv, c, id)
	setVictimRoll(t, srv, id, 51)
	dropped := endOnDrop(t, srv, zaricheID)

	killByNPC(t, srv, id)

	frames := readQuiet(c)
	holder := victimCharacter(t, srv, id)
	if holder.CursedWeaponEquipped() || holder.Karma() != 100 || holder.ProgressionValues().PKKills != 3 {
		t.Fatalf("after death: weapon %d karma %d pk %d, want none, 100, 3", holder.CursedWeaponID(), holder.Karma(), holder.ProgressionValues().PKKills)
	}
	if holder.Inventory().ItemByTemplateID(zaricheID) != nil {
		t.Fatal("Zariche still held")
	}
	assertEndedDropGone(t, srv, frames, dropped.Load(), zaricheID)
}
