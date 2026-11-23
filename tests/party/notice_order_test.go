package party

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPartyNoticesFollowDecisionOrder pins #5144 on the pool executor:
// Joiner accepts an invitation while Member, on its own queue, leaves the
// same party. Whichever change is decided first, every member sees the two
// in that order: a Joiner whose window lists Member has Member's row
// deleted only after the window arrived, and a Member who saw Joiner added
// saw it before its own window was cleared. Delivered in the opposite
// order, either keeps a ghost row.
func TestPartyNoticesFollowDecisionOrder(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}, {"Joiner", 50}}, gameservertest.WithRealPool())
	leader, member, joiner := g.players[0], g.players[1], g.players[3]
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)

	for round := range 25 {
		leader.c.Send(encodeJoinParty(joiner.name, 0))
		assertSystemMessageText(t, skipPositions(leader.c), serverpackets.SystemMessageYouInvitedS1ToParty, joiner.name)
		assertFrameOpcode(t, joiner.c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")

		joiner.c.Send(encodeAnswerJoinParty(1))
		member.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))

		// Joiner: its window comes first; when it lists Member, Member's
		// delete follows it.
		listsMember := false
		for sawWindow := false; !sawWindow || listsMember; {
			frame := joiner.c.Read()
			switch frame[0] {
			case serverpackets.OpcodePartySmallWindowAll:
				_, _, rows := readWindowAll(t, frame)
				listsMember = slices.ContainsFunc(rows, func(r windowRow) bool { return r.objectID == member.id })
				sawWindow = true
			case serverpackets.OpcodePartySmallWindowDelete:
				id, _ := readWindowDelete(t, frame)
				if id != member.id {
					continue
				}
				if !sawWindow {
					t.Fatalf("round %d: Joiner had Member's row deleted before its party window arrived", round)
				}
				listsMember = false
			}
		}

		// Member: a Joiner added to its window is added before the window
		// is cleared.
		frames := drainFrames(t, member.c)
		added, cleared := -1, -1
		for i, f := range frames {
			switch f[0] {
			case serverpackets.OpcodePartySmallWindowAdd:
				added = i
			case serverpackets.OpcodePartySmallWindowDeleteAll:
				cleared = i
			}
		}
		if cleared < 0 {
			t.Fatalf("round %d: Member's window was never cleared: %x", round, opcodes(frames))
		}
		if added > cleared {
			t.Fatalf("round %d: Member saw Joiner added after its window was cleared: %x", round, opcodes(frames))
		}

		// Back to Leader, Member and Third for the next round.
		g.quiet(t)
		joiner.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
		g.quiet(t)
		g.invite(t, 0, 1, 0)
	}
}
