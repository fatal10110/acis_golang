package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPartyAutoLootLooterNeedsRoomForEveryInstance pins #5098: three
// auto-looted weapons arrive as three instances under the multiple item
// drop rule, so Member, whose turn it is but who has room for two, is
// passed over and the turn goes on to Leader. Without the rule they arrive
// as one instance, which Member has room for.
func TestPartyAutoLootLooterNeedsRoomForEveryInstance(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		opts          []gameservertest.Option
		looter, other int
		want          int
	}{
		{name: "multiple item drop", looter: 0, other: 1, want: 3},
		{name: "single item drop", opts: []gameservertest.Option{gameservertest.WithSingleItemDrop()}, looter: 1, other: 0, want: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := append([]gameservertest.Option{gameservertest.WithAutoLoot(true)}, tt.opts...)
			g := lootGroup(t, []seat{{"Leader", 20}, {"Member", 20}}, 2, party.LootByTurn, nil, opts...)
			memberID := g.players[1].id
			limit := g.srv.PlayerInventory(t, memberID).Size() + 2
			g.srv.SetInventorySlotLimit(t, memberID, limit)

			frames := killPartyDrop(t, g, partyDropMonster(30, 3))

			looter, other := g.players[tt.looter], g.players[tt.other]
			if got := g.carried(t, tt.looter, 30); got != tt.want {
				t.Fatalf("%s carries %d weapons, want %d", looter.name, got, tt.want)
			}
			if got := g.carried(t, tt.other, 30); got != 0 {
				t.Fatalf("%s carries %d weapons, want none", other.name, got)
			}
			if got := g.persisted(t, tt.looter, 30); got != tt.want {
				t.Fatalf("%s's persisted weapons = %d, want %d", looter.name, got, tt.want)
			}
			if size := g.srv.PlayerInventory(t, memberID).Size(); size > limit {
				t.Fatalf("Member holds %d slots past its limit of %d", size, limit)
			}
			requireMessage(t, looter.name, frames[tt.looter], serverpackets.SystemMessageYouPickedUpS2S1, int32(30), int32(3))
			requireMessage(t, other.name, frames[tt.other], serverpackets.SystemMessageS1ObtainedS3S2, looter.name, int32(30), int32(3))
		})
	}
}
