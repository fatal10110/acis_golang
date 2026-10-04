package party

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// RequestJoinParty's refusals that read another system (RequestJoinParty.java
// runImpl): the target blocking everything (S1_BLOCKED_EVERYTHING) and the
// target's block list (S1_HAS_ADDED_YOU_TO_IGNORE_LIST) come first, ahead of
// the wrong-target check; a target without a client and either side in jail
// answer a plain text once the in-party check passed. Each is answered on the
// inviter's side alone, and the target is asked nothing.

const (
	detachedText = "The player you tried to invite is in offline mode."
	jailedText   = "The player you tried to invite is currently jailed."
)

func encodeBlock(typ int32, name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBlock)
	w.WriteInt32(typ)
	if typ == clientpackets.BlockAdd || typ == clientpackets.BlockRemove {
		w.WriteString(name)
	}
	return w.Bytes()
}

// character returns player i's online character.
func (g *group) character(t *testing.T, i int) *playermodel.Character {
	t.Helper()
	obj, ok := g.srv.State.Player(g.players[i].id)
	if !ok {
		t.Fatalf("%s missing from the world", g.players[i].name)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%s is %T, not an online character", g.players[i].name, obj)
	}
	return c
}

func TestInviteRefusedWhileTargetBlocksEverything(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	leader, member := g.players[0], g.players[1]
	member.c.Send(encodeBlock(clientpackets.BlockAll, ""))
	assertStaticSystemMessage(t, member.c.Read(), serverpackets.SystemMessageBlockingAll)
	g.quiet(t)

	leader.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageS1BlockedEverything, "Member")
	assertSilent(t, member.c, "target blocking everything")

	// The block is the first check: it answers even an invitation of
	// oneself.
	leader.c.Send(encodeBlock(clientpackets.BlockAll, ""))
	assertStaticSystemMessage(t, leader.c.Read(), serverpackets.SystemMessageBlockingAll)
	g.quiet(t)
	leader.c.Send(encodeJoinParty("Leader", 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageS1BlockedEverything, "Leader")
}

func TestInviteRefusedByTargetBlockList(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
	leader, member := g.players[0], g.players[1]
	member.c.Send(encodeBlock(clientpackets.BlockAdd, "Leader"))
	assertSystemMessageText(t, member.c.Read(), serverpackets.SystemMessageS1AddedToYourIgnoreList, "Leader")
	g.quiet(t)

	leader.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageS1HasAddedYouToIgnoreList, "Member")
	assertSilent(t, member.c, "target of a blocked inviter")

	// The block list is read before the party check: a blocking target
	// already in a party still answers the block.
	g.invite(t, 1, 2, 0)
	leader.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageS1HasAddedYouToIgnoreList, "Member")

	// The block is one way: the blocker may still invite the player it
	// blocks.
	member.c.Send(encodeJoinParty("Leader", 0))
	assertSystemMessageText(t, skipPositions(member.c), serverpackets.SystemMessageYouInvitedS1ToParty, "Leader")
	assertFrameOpcode(t, leader.c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty from the blocker")
}

func TestInviteRefusedWhileTargetConnectionIsGone(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	leader, member := g.players[0], g.players[1]
	// In combat, a player whose connection dropped stays in the world a
	// while before it leaves.
	g.srv.SetPlayerInCombat(t, member.id, true)
	if err := member.c.Close(); err != nil {
		t.Fatal(err)
	}
	obj, ok := g.srv.State.Player(member.id)
	if !ok {
		t.Fatal("Member left the world at once")
	}
	for deadline := time.Now().Add(5 * time.Second); !network.ClientDetached(obj); {
		if time.Now().After(deadline) {
			t.Fatal("the server never noticed Member's connection drop")
		}
		time.Sleep(time.Millisecond)
	}

	leader.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageS1, detachedText)
	if _, ok := g.srv.State.Player(member.id); !ok {
		t.Fatal("Member left the world before the invitation was refused")
	}
}

func TestInviteRefusedWhileJailed(t *testing.T) {
	t.Run("target", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
		g.character(t, 1).SetPunishment(playermodel.PunishJail, 0)
		g.players[0].c.Send(encodeJoinParty("Member", 0))
		assertSystemMessageText(t, g.players[0].c.Read(), serverpackets.SystemMessageS1, jailedText)
		assertSilent(t, g.players[1].c, "jailed target")
	})
	t.Run("inviter", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
		g.character(t, 0).SetPunishment(playermodel.PunishJail, 0)
		g.players[0].c.Send(encodeJoinParty("Member", 0))
		assertSystemMessageText(t, g.players[0].c.Read(), serverpackets.SystemMessageS1, jailedText)
		assertSilent(t, g.players[1].c, "target of a jailed inviter")
	})
	t.Run("party check first", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
		g.invite(t, 0, 1, 0)
		g.character(t, 2).SetPunishment(playermodel.PunishJail, 0)
		g.players[2].c.Send(encodeJoinParty("Member", 0))
		assertSystemMessageText(t, g.players[2].c.Read(), serverpackets.SystemMessageS1IsAlreadyInParty, "Member")
	})
}
