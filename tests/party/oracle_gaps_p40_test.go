package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// TestChannelLeaderOustsOwnParty has the leader of a three-party command
// channel name a member of its own party in RequestExOustFromMPCC
// (RequestExOustFromMPCC.java:20-67, CommandChannel.removeParty): the
// naming checks all pass, so its own party is dismissed like any other. The
// dismissed party is shown the channel closed and told it was dismissed,
// without the party-dismissed notice (the requester's party is no longer in
// the channel); the parties left are sent the party's removal from the
// list.
func TestChannelLeaderOustsOwnParty(t *testing.T) {
	t.Parallel()
	g, _ := bootAuthority(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}, {"Fifth", 30}, {"Sixth", 30}},
		authorityClan{level: 5, leads: true, skills: []int{clanImperiumID}}, 0)
	g.invite(t, 0, 1, 0)
	g.invite(t, 2, 3, 0)
	g.invite(t, 4, 5, 0)
	g.askAndAccept(t, 0, 2)
	g.askAndAccept(t, 0, 4)

	g.players[0].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExOustFromMPCC, "Member"))
	for i, p := range g.players {
		frames := drainFrames(t, p.c)
		if i < 2 {
			assertEvents(t, events(t, frames), []int{ext(subCloseMPCC), msgDismissed}, p.name+" dismissal of its own party")
			continue
		}
		assertEvents(t, events(t, frames), []int{ext(subPartyInfoMPCCU)}, p.name+" remaining party")
		assertPartyInfoUpdate(t, frames, "Leader", g.players[0].id, 2, 0)
	}
}
