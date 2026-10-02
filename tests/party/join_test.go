package party

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestInviteFormsParty pins RequestJoinParty and RequestAnswerJoinParty
// forming a party: the inviter is told whom it invited and the target gets
// AskJoinParty naming the inviter with the chosen loot rule. Accepting
// sends the inviter JoinParty(1); the new member's window lists the leader,
// the leader's window adds the member, each is told the join, and both
// refresh their full view (UserInfo to self, CharInfo to the other). Six
// seconds later, and every twelve after, each member gets every member's
// position.
func TestInviteFormsParty(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	leader, member := g.players[0], g.players[1]

	leader.c.Send(encodeJoinParty("member", 3))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "Member")
	ask := member.c.Read()
	assertFrameOpcode(t, ask, serverpackets.OpcodeAskJoinParty, "AskJoinParty")
	r := wire.NewReader(ask[1:])
	if name, loot := r.ReadString(), r.ReadInt32(); name != "Leader" || loot != 3 {
		t.Fatalf("AskJoinParty = %q loot %d, want Leader loot 3", name, loot)
	}

	member.c.Send(encodeAnswerJoinParty(1))
	memberFrames := drainFrames(t, member.c)
	leaderFrames := drainFrames(t, leader.c)

	assertOpcodes(t, leaderFrames, []byte{
		serverpackets.OpcodeJoinParty,
		serverpackets.OpcodePartySmallWindowAdd,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeCharInfo,
		serverpackets.OpcodeRelationChanged,
	}, "leader")
	if got := wire.NewReader(leaderFrames[0][1:]).ReadInt32(); got != 1 {
		t.Fatalf("JoinParty response = %d, want 1", got)
	}
	if lead, loot, row := readWindowAdd(t, leaderFrames[1]); lead != leader.id || loot != 3 || row.objectID != member.id || row.name != "Member" || row.level != 30 {
		t.Fatalf("leader PartySmallWindowAdd = leader %d loot %d row %+v", lead, loot, row)
	}
	assertSystemMessageText(t, leaderFrames[2], serverpackets.SystemMessageS1JoinedParty, "Member")

	assertOpcodes(t, memberFrames, []byte{
		serverpackets.OpcodePartySmallWindowAll,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeCharInfo,
		serverpackets.OpcodeRelationChanged,
		serverpackets.OpcodeUserInfo,
	}, "member")
	lead, loot, rows := readWindowAll(t, memberFrames[0])
	if lead != leader.id || loot != 3 || len(rows) != 1 || rows[0].objectID != leader.id || rows[0].name != "Leader" || rows[0].level != 20 {
		t.Fatalf("member PartySmallWindowAll = leader %d loot %d rows %+v", lead, loot, rows)
	}
	assertSystemMessageText(t, memberFrames[1], serverpackets.SystemMessageYouJoinedS1Party, "Leader")

	g.srv.Advance(t, 6*time.Second)
	for _, p := range g.players {
		assertPositions(t, p.c.Read(), leader.id, member.id)
	}
	g.srv.Advance(t, 12*time.Second)
	for _, p := range g.players {
		assertPositions(t, p.c.Read(), leader.id, member.id)
	}
}

func assertPositions(t *testing.T, frame []byte, ids ...int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodePartyMemberPosition, "PartyMemberPosition")
	r := wire.NewReader(frame[1:])
	if n := r.ReadInt32(); int(n) != len(ids) {
		t.Fatalf("PartyMemberPosition count = %d, want %d", n, len(ids))
	}
	for _, want := range ids {
		if got := r.ReadInt32(); got != want {
			t.Fatalf("PartyMemberPosition member = %d, want %d", got, want)
		}
		r.ReadInt32()
		r.ReadInt32()
		r.ReadInt32()
	}
}

// TestThirdMemberJoins pins Party.addPartyMember: the joining member's
// window lists every member already in, the others' windows add it, and the
// party keeps the loot rule it formed with, whatever the new invitation
// offered.
func TestThirdMemberJoins(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}})
	leader, member, third := g.players[0], g.players[1], g.players[2]
	g.invite(t, 0, 1, 1)

	leader.c.Send(encodeJoinParty("Third", 4))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "Third")
	ask := third.c.Read()
	r := wire.NewReader(ask[1:])
	if _, loot := r.ReadString(), r.ReadInt32(); loot != 1 {
		t.Fatalf("AskJoinParty loot = %d, want the party's 1", loot)
	}
	third.c.Send(encodeAnswerJoinParty(1))

	thirdFrames := drainFrames(t, third.c)
	lead, loot, rows := readWindowAll(t, thirdFrames[0])
	if lead != leader.id || loot != 1 || len(rows) != 2 || rows[0].objectID != leader.id || rows[1].objectID != member.id {
		t.Fatalf("third PartySmallWindowAll = leader %d loot %d rows %+v", lead, loot, rows)
	}
	assertSystemMessageText(t, thirdFrames[1], serverpackets.SystemMessageYouJoinedS1Party, "Leader")
	for _, p := range []player{leader, member} {
		frames := drainFrames(t, p.c)
		if p.id == leader.id {
			frames = frames[1:] // JoinParty
		}
		if _, _, row := readWindowAdd(t, frames[0]); row.objectID != third.id {
			t.Fatalf("%s PartySmallWindowAdd member = %d, want %d", p.name, row.objectID, third.id)
		}
		assertSystemMessageText(t, frames[1], serverpackets.SystemMessageS1JoinedParty, "Third")
	}
}

// TestDeclineSendsJoinPartyZero pins a refused invitation: the inviter
// gets JoinParty(0), nobody joins, and the inviter is free to invite again
// at once.
func TestDeclineSendsJoinPartyZero(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	leader, member := g.players[0], g.players[1]

	leader.c.Send(encodeJoinParty("Member", 0))
	leader.c.Read()
	member.c.Read()
	member.c.Send(encodeAnswerJoinParty(0))
	frame := leader.c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeJoinParty, "JoinParty")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
		t.Fatalf("JoinParty response = %d, want 0", got)
	}
	assertSilent(t, leader.c, "leader after the refusal")
	assertSilent(t, member.c, "member after its refusal")

	leader.c.Send(encodeJoinParty("Member", 0))
	assertSystemMessageText(t, leader.c.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "Member")
}

// TestAnswerWithoutInvitationIsSilent pins RequestAnswerJoinParty with no
// active requester: nothing happens and nothing is sent.
func TestAnswerWithoutInvitationIsSilent(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}})
	g.players[1].c.Send(encodeAnswerJoinParty(1))
	assertSilent(t, g.players[1].c, "answer with nothing pending")
	assertSilent(t, g.players[0].c, "bystander")
}
