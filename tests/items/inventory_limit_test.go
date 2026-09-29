package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPickupRefusedAtConfiguredInventoryLimit pins the player slot limit to
// the configured MaximumSlotsForNoDwarf: a character holding that many
// stacks is told its limit on entry, a new stack on the ground is refused
// with ActionFailed then SlotsFull and stays there, and a pickup that only
// merges into a held stack still goes through.
func TestPickupRefusedAtConfiguredInventoryLimit(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithInventorySlots(2, 3),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 30, 1)
	srv.GiveItem(t, objID, item.AdenaID, 10)
	burst := startInWorld(t, c)

	storage := burst[1]
	r := wire.NewReader(storage[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExStorageMaxCount {
		t.Fatalf("enter-world extended frame = %#x, want ExStorageMaxCount", sub)
	}
	if got := r.ReadInt32(); got != 2 {
		t.Fatalf("ExStorageMaxCount inventory limit = %d, want the configured 2", got)
	}

	srv.SeedGroundItem(t, 0, item.AdenaID, 5, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	adenaID := soleGroundObjectID(t, srv)
	c.Send(encodeAction(adenaID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
	drainUntilQuiet(t, c)
	if _, ok := srv.State.Object(adenaID); ok {
		t.Fatal("stackable adena stayed on the ground although it needs no new slot")
	}

	srv.SeedGroundItem(t, 0, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	swordID := soleGroundObjectID(t, srv)
	c.Send(encodeAction(swordID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "slots-full lead")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSlotsFull)
	barrier(t, c)
	if _, ok := srv.State.Object(swordID); !ok {
		t.Fatal("ground weapon left the ground for a full inventory")
	}
	if n := carriedCount(t, srv, objID, 30); n != 1 {
		t.Fatalf("carried weapons after the refused pickup = %d, want 1", n)
	}
}
