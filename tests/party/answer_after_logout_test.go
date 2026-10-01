package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestAcceptAfterInviterLeftFormsNothing pins a deliberate difference from
// the reference: an acceptance that comes after its inviter left the world
// forms no party with the departed character. The target hears nothing,
// is in no party, and is free to be invited again at once.
func TestAcceptAfterInviterLeftFormsNothing(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
	leader, member, third := g.players[0], g.players[1], g.players[2]

	leader.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "Member")
	assertFrameOpcode(t, member.c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")

	leader.c.Send(encodeSingle(clientpackets.OpcodeLogout))
	g.srv.AdvanceUntil(t, "leader leaves the world", func() bool {
		_, ok := g.srv.State.Player(leader.id)
		return !ok
	})
	drainFrames(t, member.c)
	drainFrames(t, third.c)

	member.c.Send(encodeAnswerJoinParty(1))
	assertSilent(t, member.c, "target accepting a departed inviter")
	assertSilent(t, third.c, "bystander")

	// In no party and holding no request: a fresh invitation reaches it.
	third.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, third.c.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "Member")
	assertFrameOpcode(t, member.c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")
	member.c.Send(encodeAnswerJoinParty(1))
	frames := drainFrames(t, member.c)
	lead, _, rows := readWindowAll(t, frames[indexOf(frames, serverpackets.OpcodePartySmallWindowAll)])
	if lead != third.id || len(rows) != 1 || rows[0].objectID != third.id {
		t.Fatalf("PartySmallWindowAll leader %d rows %+v, want a party of Third and Member only", lead, rows)
	}
}
