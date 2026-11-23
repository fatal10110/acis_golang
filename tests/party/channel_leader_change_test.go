package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// TestChannelAnswerAfterLeaderChange (#5106): Leader invites Other's party
// into a command channel it would form, then one side hands its party over
// before Other accepts. The invitation no longer stands: no channel forms,
// nobody is answered, and Leader keeps its Strategy Guide.
func TestChannelAnswerAfterLeaderChange(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		from, to int
	}{
		{"requester handed its party over", 0, 1},
		{"answerer handed its party over", 2, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, _ := bootAuthority(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}},
				authorityClan{level: 5, leads: true}, 1)
			g.invite(t, 0, 1, 0)
			g.invite(t, 2, 3, 0)

			g.players[0].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, "Other"))
			assertSystemMessageText(t, skipPositions(g.players[2].c), msgConfirmFrom, "Leader")
			g.quiet(t)
			g.players[tc.from].c.Send(encodeExtendedName(clientpackets.OpcodeRequestChangePartyLeader, g.players[tc.to].name))
			g.quiet(t)

			g.players[2].c.Send(encodeExtendedInt(clientpackets.OpcodeRequestExAcceptJoinMPCC, 1))
			g.srv.InventoryUpdates.Tick()
			for _, p := range g.players {
				assertSilent(t, p.c, p.name+" after a stale acceptance")
			}
			if n := heldGuides(t, g); n != 1 {
				t.Fatalf("guides after a stale acceptance = %d, want 1", n)
			}
		})
	}
}
