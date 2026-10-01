package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestInviteRefusals pins RequestJoinParty's refusals, each answered on the
// inviter's side alone: an unknown name, oneself, a target already in a
// party, a member who does not lead, and a full party.
func TestInviteRefusals(t *testing.T) {
	t.Run("unknown name", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
		g.players[0].c.Send(encodeJoinParty("Nobody", 0))
		assertStaticSystemMessage(t, g.players[0].c.Read(), serverpackets.SystemMessageFirstSelectUserToInviteToParty)
		assertSilent(t, g.players[1].c, "bystander")
	})
	t.Run("self", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
		g.players[0].c.Send(encodeJoinParty("Leader", 0))
		assertStaticSystemMessage(t, g.players[0].c.Read(), serverpackets.SystemMessageYouHaveInvitedTheWrongTarget)
	})
	t.Run("target in a party", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
		g.invite(t, 0, 1, 0)
		g.players[2].c.Send(encodeJoinParty("Member", 0))
		assertSystemMessageText(t, g.players[2].c.Read(), serverpackets.SystemMessageS1IsAlreadyInParty, "Member")
		assertSilent(t, g.players[1].c, "target already in a party")
	})
	t.Run("not the leader", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
		g.invite(t, 0, 1, 0)
		g.players[1].c.Send(encodeJoinParty("Third", 0))
		assertStaticSystemMessage(t, g.players[1].c.Read(), serverpackets.SystemMessageOnlyLeaderCanInvite)
		assertSilent(t, g.players[2].c, "target of a member's invitation")
	})
	t.Run("unknown loot rule", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
		g.players[0].c.Send(encodeJoinParty("Member", 5))
		assertSilent(t, g.players[0].c, "inviter offering no such rule")
		assertSilent(t, g.players[1].c, "target of a rule-less invitation")
	})
}

// TestInviteWaitsForReply pins the shared requester slot: while one
// invitation waits, the inviter can invite no one else, the invited player
// can be asked for nothing else, a trade request to it is refused as busy,
// and it cannot answer the invitation as a trade request.
func TestInviteWaitsForReply(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
	leader, member, third := g.players[0], g.players[1], g.players[2]

	leader.c.Send(encodeJoinParty("Member", 0))
	leader.c.Read()
	member.c.Read()

	leader.c.Send(encodeJoinParty("Third", 0))
	assertStaticSystemMessage(t, leader.c.Read(), serverpackets.SystemMessageWaitingForAnotherReply)
	third.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, third.c.Read(), serverpackets.SystemMessageS1IsBusyTryLater, "Member")

	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(member.id)
	third.c.Send(w.Bytes())
	assertSystemMessageText(t, third.c.Read(), serverpackets.SystemMessageS1IsBusyTryLater, "Member")

	// A trade answer finds no trade request to answer; the invitation
	// stays pending.
	w = wire.NewPacketWriter(clientpackets.OpcodeAnswerTradeRequest)
	w.WriteInt32(1)
	member.c.Send(w.Bytes())
	drainFrames(t, member.c)
	assertSilent(t, leader.c, "inviter after a trade answer")

	member.c.Send(encodeAnswerJoinParty(1))
	assertFrameOpcode(t, leader.c.Read(), serverpackets.OpcodeJoinParty, "JoinParty")
}

// TestPartyFull pins the nine-member cap: a full party's leader is told
// the party is full.
func TestPartyFull(t *testing.T) {
	seats := []seat{{"Leader", 20}}
	for _, name := range []string{"Mone", "Mtwo", "Mthree", "Mfour", "Mfive", "Msix", "Mseven", "Meight", "Mnine"} {
		seats = append(seats, seat{name, 20})
	}
	g := bootGroup(t, seats)
	for i := 1; i < 9; i++ {
		g.invite(t, 0, i, 0)
	}
	g.quiet(t)
	g.players[0].c.Send(encodeJoinParty("Mnine", 0))
	assertStaticSystemMessage(t, skipPositions(g.players[0].c), serverpackets.SystemMessagePartyFull)
	assertSilent(t, g.players[9].c, "target of a full party's invitation")
}
