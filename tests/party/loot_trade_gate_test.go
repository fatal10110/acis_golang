package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// Reference (#4972): PlayerAI.thinkPickUp checks the picker's own capacity,
// only under ITEM_LOOTER inside a party, before the open trade
// (PlayerAI.java:338-352). Under the other rules a full picker with an open
// trade is refused for the trade.

// openLootTrade opens a trade between the group's players a and b.
func (g *group) openLootTrade(t *testing.T, a, b int) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(g.players[b].id)
	g.players[a].c.Send(w.Bytes())
	assertFrameOpcode(t, g.players[b].c.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	w = wire.NewPacketWriter(clientpackets.OpcodeAnswerTradeRequest)
	w.WriteInt32(1)
	g.players[b].c.Send(w.Bytes())
	g.quiet(t)
}

// TestPartyPickupTradeGateFollowsCapacity: a full trading picker hears
// SLOTS_FULL under finders-keepers, and the trade refusal under random,
// where its own room is not checked. Either way the item stays put.
func TestPartyPickupTradeGateFollowsCapacity(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		rule      party.LootRule
		want, not int
	}{
		"finders keepers": {party.LootFindersKeepers, serverpackets.SystemMessageSlotsFull, serverpackets.SystemMessageCannotPickupOrUseItemTrading},
		"random":          {party.LootRandom, serverpackets.SystemMessageCannotPickupOrUseItemTrading, serverpackets.SystemMessageSlotsFull},
	} {
		t.Run(name, func(t *testing.T) {
			g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, tc.rule, nil)
			g.openLootTrade(t, 0, 1)
			g.srv.SetInventorySlotLimit(t, g.players[0].id, 0)

			frames := g.pickUp(t, 0, 0, lootPotionID, 2)
			if len(frames[0]) == 0 || frames[0][0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("Leader's refusal does not open with ActionFailed: %d frames", len(frames[0]))
			}
			requireMessage(t, "Leader", frames[0], tc.want)
			requireNoMessage(t, "Leader", frames[0], tc.not)
			if len(g.srv.GroundItems.Snapshots(nil)) != 1 {
				t.Fatal("a refused pickup left the ground")
			}
			for i := range 2 {
				if got := g.carried(t, i, lootPotionID); got != 0 {
					t.Fatalf("%s carries %d potions, want none", g.players[i].name, got)
				}
			}
		})
	}
}
