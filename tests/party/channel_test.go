package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// twoParties boots four players in two parties: Leader with Member, Other
// with Fourth.
func twoParties(t *testing.T) *group {
	t.Helper()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}})
	g.invite(t, 0, 1, 0)
	g.invite(t, 2, 3, 0)
	return g
}

// TestChannelInviteRefusals pins RequestExAskJoinMPCC up to the authority
// check: an unknown name, a requester or target without a party, and both
// in one party are ignored; a member who leads nothing cannot invite; and a
// party leader who leads no level 5 clan is refused. Nobody is asked.
func TestChannelInviteRefusals(t *testing.T) {
	g := twoParties(t)
	leader, member, other := g.players[0], g.players[1], g.players[2]

	for _, tc := range []struct {
		who  player
		name string
		what string
	}{
		{leader, "Nobody", "unknown name"},
		{leader, "Member", "own party"},
	} {
		tc.who.c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, tc.name))
		assertSilent(t, tc.who.c, tc.what)
	}

	member.c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, "Other"))
	assertStaticSystemMessage(t, skipPositions(member.c), serverpackets.SystemMessageCannotInviteToCommandChannel)

	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, "Fourth"))
	assertStaticSystemMessage(t, skipPositions(leader.c), serverpackets.SystemMessageCommandChannelOnlyByLevel5ClanLeader)
	for _, p := range g.players[2:] {
		frames := drainFrames(t, p.c)
		if indexOf(frames, serverpackets.OpcodeExtended) >= 0 {
			t.Fatalf("%s frames = %x, want no channel invitation", p.name, opcodes(frames))
		}
	}

	// With nothing pending, an answer does nothing.
	other.c.Send(encodeExtendedInt(clientpackets.OpcodeRequestExAcceptJoinMPCC, 1))
	assertSilent(t, other.c, "channel answer with nothing pending")
}

// TestChannelInviteWithoutParty pins RequestExAskJoinMPCC from a player in
// no party: ignored without an answer.
func TestChannelInviteWithoutParty(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
	g.invite(t, 1, 2, 0)
	g.players[0].c.Send(encodeExtendedName(clientpackets.OpcodeRequestExAskJoinMPCC, "Member"))
	assertSilent(t, g.players[0].c, "partyless channel invitation")
}

// TestChannelOustRefusals pins RequestExOustFromMPCC's refusals: an unknown
// name, oneself, a target in no party, and a requester leading no channel.
func TestChannelOustRefusals(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Loner", 40}})
	g.invite(t, 0, 1, 0)
	leader := g.players[0]
	for _, tc := range []struct {
		name string
		want int
	}{
		{"Nobody", serverpackets.SystemMessageTargetCantFound},
		{"Leader", serverpackets.SystemMessageInvalidTarget},
		{"Loner", serverpackets.SystemMessageInvalidTarget},
		{"Member", serverpackets.SystemMessageNotAuthorizedToDoThat},
	} {
		leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestExOustFromMPCC, tc.name))
		assertStaticSystemMessage(t, skipPositions(leader.c), tc.want)
	}
}

// TestChannelPartyMembersInfo pins RequestExMPCCShowPartyMembersInfo: any
// online party member's id lists its party's members, in join order, with
// their class; a player in no party lists nothing.
func TestChannelPartyMembersInfo(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Loner", 40}})
	g.invite(t, 0, 1, 0)
	loner := g.players[2]

	loner.c.Send(encodeExtendedInt(clientpackets.OpcodeRequestExMPCCShowPartyMembersInfo, g.players[1].id))
	frame := loner.c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "ExMPCCShowPartyMemberInfo")
	r := wire.NewReader(frame[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExMPCCShowPartyMemberInfo {
		t.Fatalf("sub-opcode = %#x, want %#x", sub, serverpackets.OpcodeExMPCCShowPartyMemberInfo)
	}
	if n := r.ReadInt32(); n != 2 {
		t.Fatalf("member count = %d, want 2", n)
	}
	for _, want := range g.players[:2] {
		if name, id := r.ReadString(), r.ReadInt32(); name != want.name || id != want.id {
			t.Fatalf("member = %q %d, want %q %d", name, id, want.name, want.id)
		}
		r.ReadInt32()
	}

	g.players[0].c.Send(encodeExtendedInt(clientpackets.OpcodeRequestExMPCCShowPartyMembersInfo, loner.id))
	assertSilent(t, g.players[0].c, "members of no party")
}
