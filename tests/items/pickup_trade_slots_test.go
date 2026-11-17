package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for the tests below (#4972): PlayerAI.thinkPickUp checks the
// picker's capacity (outside a party or under ITEM_LOOTER) before the open
// trade, and both before the loot lock and the herb handler
// (PlayerAI.java:338-360). A full picker with an open trade hears
// SLOTS_FULL, never CANNOT_PICKUP_OR_USE_ITEM_WHILE_TRADING.

// bootTradingPicker boots a player in an open trade with a second player,
// with templateID lying at its feet, and returns the ground item's id.
func bootTradingPicker(t *testing.T, templateID int32, opts ...gameservertest.Option) (*gameservertest.Server, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	second := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)
	openTrade(t, c, observer, objID, second.ID)

	srv.SeedGroundItem(t, objID, templateID, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, observer)
	return srv, objID, soleGroundObjectID(t, srv)
}

// TestPickupSlotsFullWinsOverOpenTrade: a full picker with an open trade is
// refused for its full inventory, and the item stays on the ground.
func TestPickupSlotsFullWinsOverOpenTrade(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootTradingPicker(t, 30)
	c := srv.Client
	srv.SetInventorySlotLimit(t, objID, 1)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "slots-full lead")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSlotsFull)
	assertNoFrame(t, c, "after the slots-full refusal")
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("ground item removed after a slots-full pickup attempt")
	}
}

// TestHerbPickupRejectedWhileTrading: a herb needs no slot, so even past
// its limit a trading picker hears the trade refusal, and the herb stays
// unused on the ground.
func TestHerbPickupRejectedWhileTrading(t *testing.T) {
	t.Parallel()
	srv, objID, groundID := bootTradingPicker(t, 8600, gameservertest.WithSkills(herbSkills(t)))
	c := srv.Client
	srv.SetInventorySlotLimit(t, objID, 0)

	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "trading herb pickup lead")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotPickupOrUseItemTrading)
	assertNoFrame(t, c, "after the trading refusal")
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("herb used while its picker was trading")
	}
}
