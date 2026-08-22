package lifecycle

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBootWiringReachesEverySubsystem drives each wired subsystem through
// its client-visible surface in one booted stack: the roster's restored rows
// reach the ItemList, the ground-items task receives a live drop, and the
// lazy persistence task holds the resulting mutation until it is drained. A
// wiring regression — a provide returning nil that nothing notices — shows
// up as a missing frame, an absent world object, or a lost row.
func TestBootWiringReachesEverySubsystem(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)

	frames := startInWorld(t, c)

	entries := readItemListEntries(t, burstFrame(t, frames, serverpackets.OpcodeItemList))
	e := findItemListEntry(entries, adena)
	if e == nil {
		t.Fatalf("ItemList entries = %+v, want the seeded adena row %d", entries, adena)
	}
	if e.count != 100 || e.equipped != 0 {
		t.Fatalf("seeded adena row = count %d equipped %d, want 100/0", e.count, e.equipped)
	}

	c.Send(encodeRequestDropItem(adena, 40, spawnX, spawnY, spawnZ))
	groundID := readDropItemGroundID(t, c.Read(), objID, item.AdenaID, 40)

	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatalf("world.Object(%d) missing for dropped item", groundID)
	}
	if got := srv.GroundItems.Len(); got != 1 {
		t.Fatalf("GroundItems.Len() after drop = %d, want 1", got)
	}

	srv.FlushItems(t)
	counts := persistedAdena(t, srv, objID)
	if len(counts) != 1 || counts[0] != 60 {
		t.Fatalf("persisted adena stacks after drop = %v, want one stack of 60", counts)
	}
}
