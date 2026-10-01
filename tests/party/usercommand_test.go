package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// User command ids the client sends for /partyinfo and the command channel
// commands.
const (
	cmdPartyInfo         = 81
	cmdChannelDelete     = 93
	cmdChannelLeave      = 96
	cmdChannelListUpdate = 97
)

func encodeUserCommand(id int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUserCommand)
	w.WriteInt32(id)
	return w.Bytes()
}

// TestPartyInfoCommand pins /partyinfo (PartyInfo.useUserCommand): the
// party information header, the loot rule's message (1031 + rule: random
// including spoil is 1033), the leader's name, "Members: n/9" as a plain
// text message, then the footer, to the asker only. Out of a party it says
// nothing.
func TestPartyInfoCommand(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Loner", 40}})
	g.invite(t, 0, 1, 2)
	member, loner := g.players[1], g.players[2]

	loner.c.Send(encodeUserCommand(cmdPartyInfo))
	assertSilent(t, loner.c, "/partyinfo out of a party")

	member.c.Send(encodeUserCommand(cmdPartyInfo))
	assertStaticSystemMessage(t, skipPositions(member.c), serverpackets.SystemMessagePartyInformation)
	assertStaticSystemMessage(t, skipPositions(member.c), 1033)
	assertSystemMessageText(t, skipPositions(member.c), serverpackets.SystemMessagePartyLeaderS1, "Leader")
	assertSystemMessageText(t, skipPositions(member.c), serverpackets.SystemMessageS1, "Members: 2/9")
	assertStaticSystemMessage(t, skipPositions(member.c), serverpackets.SystemMessageFriendListFooter)
	assertSilent(t, member.c, "after /partyinfo")
	assertSilent(t, g.players[0].c, "the leader, when a member asks /partyinfo")
}

// TestChannelCommandsOutsideAChannel pins the command channel commands'
// silent refusals (ChannelDelete, ChannelLeave, ChannelListUpdate): a
// player in no party, a party member who leads nothing, and a party
// leader whose party is in no channel get no answer. No channel can form
// until clans can authorize one (#3158); the commands' channel paths are
// pinned on the registry (internal/gameserver/party).
func TestChannelCommandsOutsideAChannel(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Loner", 40}})
	g.invite(t, 0, 1, 0)
	for _, p := range g.players {
		for _, id := range []int32{cmdChannelDelete, cmdChannelLeave, cmdChannelListUpdate} {
			p.c.Send(encodeUserCommand(id))
			assertSilent(t, p.c, p.name+" user command")
		}
	}
}
