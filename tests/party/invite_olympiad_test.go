package party

import (
	"testing"

	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// RequestJoinParty turns an invitation away without a word when either
// side competes in an Olympiad match (RequestJoinParty.java runImpl,
// `target.isInOlympiadMode() || requestor.isInOlympiadMode()`). The check
// comes after the jail refusal and before the pending-request checks, and
// it leaves nothing pending: the same invitation goes through once the
// match is over.

func TestInviteSilentlyRefusedInOlympiad(t *testing.T) {
	for _, tc := range []struct {
		name       string
		competitor int
	}{
		{"target", 1},
		{"inviter", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
			leader, member := g.players[0], g.players[1]
			g.srv.SetPlayerOlympiadMode(t, g.players[tc.competitor].id, true)

			leader.c.Send(encodeJoinParty("Member", 0))
			assertSilent(t, leader.c, "inviter of an Olympiad invitation")
			assertSilent(t, member.c, "target of an Olympiad invitation")

			// Nothing was left pending: out of the match, the invitation
			// is asked and accepted.
			g.srv.SetPlayerOlympiadMode(t, g.players[tc.competitor].id, false)
			g.invite(t, 0, 1, 0)
			leader.c.Send(encodeJoinParty("Member", 0))
			assertSystemMessageText(t, skipPositions(leader.c), serverpackets.SystemMessageS1IsAlreadyInParty, "Member")
		})
	}
}

func TestInviteOlympiadRefusalOrder(t *testing.T) {
	t.Run("party check first", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
		g.invite(t, 0, 1, 0)
		g.srv.SetPlayerOlympiadMode(t, g.players[2].id, true)
		g.players[2].c.Send(encodeJoinParty("Member", 0))
		assertSystemMessageText(t, skipPositions(g.players[2].c), serverpackets.SystemMessageS1IsAlreadyInParty, "Member")
	})
	t.Run("jail first", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
		g.character(t, 1).SetPunishment(playermodel.PunishJail, 0)
		g.srv.SetPlayerOlympiadMode(t, g.players[0].id, true)
		g.players[0].c.Send(encodeJoinParty("Member", 0))
		assertSystemMessageText(t, g.players[0].c.Read(), serverpackets.SystemMessageS1, jailedText)
	})
	t.Run("before the pending request", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
		leader := g.players[0]
		// Leader's invitation of Third is still unanswered, which would
		// answer WAITING_FOR_ANOTHER_REPLY to a second invitation.
		leader.c.Send(encodeJoinParty("Third", 0))
		assertSystemMessageText(t, skipPositions(leader.c), serverpackets.SystemMessageYouInvitedS1ToParty, "Third")
		assertFrameOpcode(t, g.players[2].c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")
		g.quiet(t)

		g.srv.SetPlayerOlympiadMode(t, g.players[1].id, true)
		leader.c.Send(encodeJoinParty("Member", 0))
		assertSilent(t, leader.c, "inviter busy with another invitation of an Olympiad competitor")
		assertSilent(t, g.players[1].c, "Olympiad competitor")
	})
}
