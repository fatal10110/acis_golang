package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for the tests below: PlayerAI.thinkPickUp answers a pickup with
// the first gate that refuses, in this order: the picker's capacity
// (SLOTS_FULL), an open trade (CANNOT_PICKUP_OR_USE_ITEM_WHILE_TRADING),
// the drop's loot lock (FAILED_TO_PICKUP_S1 and kin). A picker refused by
// two gates hears only the earlier one.

// bootLockedPicker boots a picker holding one sword, with a sword of
// another player's drop at its feet, and returns the server, the picker's
// id, the other player's id and the ground item's id. With trading, the two
// players have an open trade.
func bootLockedPicker(t *testing.T, trading bool) (srv *gameservertest.Server, objID, otherID, groundID int32) {
	t.Helper()
	srv = gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID = srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	other := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	otherID = other.ID
	observer := srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)
	if trading {
		openTrade(t, c, observer, objID, otherID)
	}
	srv.SeedGroundItem(t, otherID, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, observer)
	return srv, objID, otherID, soleGroundObjectID(t, srv)
}

// assertRefusedBy sends the pickup click and fails unless the picker hears
// ActionFailed then the static message want and nothing else, and the item
// stays on the ground.
func assertRefusedBy(t *testing.T, srv *gameservertest.Server, groundID int32, want int, what string) {
	t.Helper()
	c := srv.Client
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, what+" lead")
	assertStaticSystemMessage(t, c.Read(), want)
	assertNoFrame(t, c, "after the refusal: "+what)
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatalf("ground item removed by a refused pickup: %s", what)
	}
}

// TestPickupSlotsFullWinsOverLootLock: a full picker clicking another
// player's locked drop hears SLOTS_FULL, not the failed-pickup notice.
func TestPickupSlotsFullWinsOverLootLock(t *testing.T) {
	t.Parallel()
	srv, objID, _, groundID := bootLockedPicker(t, false)
	srv.SetInventorySlotLimit(t, objID, 1)
	assertRefusedBy(t, srv, groundID, serverpackets.SystemMessageSlotsFull, "full picker, locked drop")
}

// TestPickupTradeWinsOverLootLock: a picker with room and an open trade
// clicking another player's locked drop hears the trade refusal, not the
// failed-pickup notice.
func TestPickupTradeWinsOverLootLock(t *testing.T) {
	t.Parallel()
	srv, _, _, groundID := bootLockedPicker(t, true)
	assertRefusedBy(t, srv, groundID, serverpackets.SystemMessageCannotPickupOrUseItemTrading, "trading picker, locked drop")
}

// TestPickupSlotsFullWinsOverTradeAndLootLock: a picker refused by all three
// gates hears only SLOTS_FULL.
func TestPickupSlotsFullWinsOverTradeAndLootLock(t *testing.T) {
	t.Parallel()
	srv, objID, _, groundID := bootLockedPicker(t, true)
	srv.SetInventorySlotLimit(t, objID, 1)
	assertRefusedBy(t, srv, groundID, serverpackets.SystemMessageSlotsFull, "full trading picker, locked drop")
}
